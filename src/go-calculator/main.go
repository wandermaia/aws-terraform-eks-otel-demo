// Package main implementa uma API REST de calculadora que realiza operações de soma
// entre dois inteiros. As operações são registradas em um banco de dados MySQL e as
// métricas de uso são expostas para o Prometheus via endpoint /metrics.
// A aplicação também é instrumentada com OpenTelemetry, enviando traces para um
// OpenTelemetry Collector via gRPC.
//
// A API expõe os seguintes endpoints:
//   - GET  /backend  — verifica se a API está em execução
//   - POST /backend  — realiza a soma de dois operadores inteiros
//   - GET  /metrics  — expõe métricas no formato Prometheus
//
// As seguintes variáveis de ambiente devem ser configuradas:
//   - DB_USER         — usuário do banco de dados
//   - DB_PASSWORD     — senha do banco de dados
//   - DB_HOST         — host (e porta) do banco de dados (ex: localhost:3306)
//   - DB_NAME         — nome do banco de dados
//   - OTEL_EXPORTER_OTLP_ENDPOINT — endereço do OpenTelemetry Collector (ex: otel-collector:4317)
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Request representa o corpo da requisição POST esperado pela API.
// O campo Nome identifica o usuário que realizou a operação,
// enquanto Operador1 e Operador2 são os valores inteiros a serem somados.
type Request struct {
	Nome      string `json:"nome"`
	Operador1 int    `json:"operador1"`
	Operador2 int    `json:"operador2"`
}

// Response representa o corpo da resposta retornado pela API após uma operação de soma bem-sucedida.
type Response struct {
	Resultado int    `json:"resultado"`
	Piada     string `json:"piada"`
}

// jokeAPIResponse representa o subconjunto do JSON retornado pelo joke-factor que
// esta aplicação consome.
type jokeAPIResponse struct {
	Value string `json:"value"`
}

// db é a conexão global com o banco de dados MySQL, inicializada por initDB.
var db *sql.DB

// dbName e dbHost armazenam as coordenadas do banco para uso nos atributos dos spans.
var dbName, dbHost string

// jokeFactorURL é o endereço do serviço joke-factor, lido da variável de ambiente
// JOKE_FACTOR_URL na inicialização.
var jokeFactorURL string

// tracer é o tracer global do OpenTelemetry utilizado para criar spans na aplicação.
var tracer trace.Tracer

// totalGets é um contador Prometheus que registra o número total de requisições
// GET recebidas no endpoint /backend
var (
	totalGets = promauto.NewCounter(prometheus.CounterOpts{
		Name: "go_calculator_total_gets",
		Help: "Numero total de GETs processados.",
	})
)

// totalPost é um contador Prometheus que registra o número total de requisições
// POST recebidas no endpoint /backend.
var (
	totalPost = promauto.NewCounter(prometheus.CounterOpts{
		Name: "go_calculator_total_posts",
		Help: "Numero total de POSTs processados.",
	})
)

// totalErros é um contador Prometheus que registra o número total de erros
// ocorridos durante o processamento das requisições (ex: JSON inválido, nome curto,
// falha no banco de dados).
var (
	totalErros = promauto.NewCounter(prometheus.CounterOpts{
		Name: "go_calculator_total_errors",
		Help: "Numero total de erros processados",
	})
)

// initTracer configura e inicializa o TracerProvider do OpenTelemetry, exportando
// traces para o OpenTelemetry Collector via gRPC. O endereço do collector é lido
// da variável de ambiente OTEL_EXPORTER_OTLP_ENDPOINT (ex: otel-collector:4317).
// Retorna uma função de shutdown que deve ser chamada com defer para garantir o
// envio de todos os spans pendentes antes do encerramento da aplicação.
func initTracer(ctx context.Context) (func(context.Context) error, error) {
	collectorAddr := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if collectorAddr == "" {
		collectorAddr = "localhost:4317"
		log.Println("OTEL_EXPORTER_OTLP_ENDPOINT não definido, usando padrão: ", collectorAddr)
	}

	conn, err := grpc.NewClient(collectorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar conexão gRPC com o collector: %w", err)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar exportador OTLP: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("go-calculator"),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar resource do OpenTelemetry: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	// Registra o propagador W3C TraceContext + Baggage globalmente.
	// Isso permite que a API extraia o contexto de trace injetado pelo frontend
	// nos headers HTTP e crie spans filhos dentro do mesmo trace distribuído.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = otel.Tracer("go-calculator")

	log.Printf("OpenTelemetry inicializado, enviando traces para %s", collectorAddr)
	return tp.Shutdown, nil
}

// initDB inicializa a conexão com o banco de dados MySQL utilizando as variáveis
// de ambiente DB_USER, DB_PASSWORD, DB_HOST e DB_NAME. Caso a conexão falhe, a
// aplicação é encerrada com log.Fatalf.
//
// Além disso, garante que a tabela "operacoes" existe no banco, criando-a caso
// necessário. A tabela armazena o nome do usuário, os dois operandos e o timestamp
// da operação.
func initDB() {
	dbHost = os.Getenv("DB_HOST")
	dbName = os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		dbHost,
		dbName,
	)

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Erro ao conectar ao banco de dados: %v", err)
	}

	log.Println("Banco de dados conectado com sucesso")

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS operacoes (
		id INT AUTO_INCREMENT PRIMARY KEY,
		nome VARCHAR(255) NOT NULL,
		operador1 INT NOT NULL,
		operador2 INT NOT NULL,
		data_execucao TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatalf("Erro ao criar tabela: %v", err)
	}

	log.Println("Tabela operacoes verificada/criada com sucesso")
}

// fetchJoke chama o serviço joke-factor para obter uma piada do Chuck Norris,
// propagando o contexto de trace distribuído via header traceparent.
// Retorna o texto da piada ou erro.
func fetchJoke(ctx context.Context) (string, error) {
	ctx, span := tracer.Start(ctx, "http.get_joke",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.method", "GET"),
			attribute.String("http.url", jokeFactorURL),
			attribute.String("server.address", "joke-factor"),
			attribute.Int("server.port", 8000),
			attribute.String("peer.service", "joke-factor"),
		),
	)
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jokeFactorURL, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("erro ao criar requisição para joke-factor: %w", err)
	}

	// Injeta o traceparent no header para que o joke-factor continue o mesmo trace.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("erro ao chamar joke-factor: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("joke-factor retornou status %d", resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	var jokeResp jokeAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&jokeResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("erro ao decodificar resposta do joke-factor: %w ", err)
	}

	span.SetStatus(codes.Ok, "")
	return jokeResp.Value, nil
}

// getHandler trata requisições GET no endpoint /backend.
// Serve como health check simples, retornando HTTP 200 com a mensagem "API em execução!".
// Incrementa o contador Prometheus totalGets a cada chamada.
func getHandler(w http.ResponseWriter, r *http.Request) {
	totalGets.Inc()
	log.Println("Requisição GET recebida em /backend")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("API em execução!"))
}

// postHandler trata requisições POST no endpoint /backend.
// Espera um corpo JSON no formato definido por Request, realiza a soma de Operador1
// e Operador2, persiste a operação no banco de dados e retorna o resultado em JSON
// conforme a estrutura Response.
//
// O handler extrai o contexto de trace distribuído dos headers HTTP recebidos
// (header "traceparent" no formato W3C TraceContext), permitindo que os spans
// criados aqui sejam filhos do trace iniciado pelo frontend.
//
// Retorna HTTP 400 (Bad Request) se:
//   - o corpo da requisição não for um JSON válido
//   - o campo Nome tiver menos de 3 caracteres
//
// Retorna HTTP 500 (Internal Server Error) se houver falha ao persistir no banco.
// Incrementa os contadores Prometheus totalPost e, em caso de erro, totalErros.
func postHandler(w http.ResponseWriter, r *http.Request) {
	totalPost.Inc()
	log.Println("Requisição POST recebida em /backend")

	// Extrai o contexto de trace distribuído dos headers HTTP (header "traceparent").
	// O span criado a seguir será filho do trace iniciado pelo frontend.
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	r = r.WithContext(ctx)

	var req Request
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		totalErros.Inc()
		log.Printf("Erro ao decodificar JSON: %v", err)
		http.Error(w, "Erro ao processar dados para soma. São aceitos apenas números inteiros.", http.StatusBadRequest)
		return
	}

	if len(req.Nome) < 3 {
		totalErros.Inc()
		log.Println("Nome inválido recebido!")
		http.Error(w, "Nome deve ter pelo menos 3 caracteres", http.StatusBadRequest)
		return
	}

	resultado := req.Operador1 + req.Operador2
	log.Printf("Operação realizada: %d + %d = %d", req.Operador1, req.Operador2, resultado)

	const insertOperacoes = "INSERT INTO operacoes (nome, operador1, operador2, data_execucao) VALUES (?, ?, ?, ?)"

	// Span filho para rastrear a gravação no banco de dados.
	dbCtx, dbCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dbCancel()
	dbCtx, span := tracer.Start(dbCtx, "db.inserir_operacao",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "mysql"),
			attribute.String("db.name", dbName),
			attribute.String("db.server.address", dbHost),
			attribute.String("db.operation", "INSERT"),
			attribute.String("db.sql.table", "operacoes"),
			attribute.String("db.statement", insertOperacoes),
			attribute.String("usuario", req.Nome),
			attribute.Int("operador1", req.Operador1),
			attribute.Int("operador2", req.Operador2),
			attribute.Int("resultado", resultado),
		),
	)

	_, err := db.ExecContext(dbCtx, insertOperacoes,
		req.Nome, req.Operador1, req.Operador2, time.Now())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		totalErros.Inc()
		log.Printf("Erro ao inserir no banco de dados: %v ", err)
		http.Error(w, "Erro ao inserir no banco de dados", http.StatusInternalServerError)
		return
	}
	span.SetStatus(codes.Ok, "")
	span.End()
	log.Println("Operação registrada no banco de dados com sucesso")

	piada, err := fetchJoke(ctx)
	if err != nil {
		totalErros.Inc()
		log.Printf("Erro ao buscar piada: %v", err)
		http.Error(w, "Erro ao buscar piada do Chuck Norris", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Resultado: resultado, Piada: piada})
}

// main é o ponto de entrada da aplicação. Inicializa o OpenTelemetry, o banco de
// dados, configura o roteador HTTP com os endpoints da API e inicia o servidor na
// porta 7000.
//
// Rotas registradas:
//   - GET  /backend  → getHandler
//   - POST /backend  → postHandler
//   - GET  /metrics  → handler do Prometheus
func main() {
	log.Println("Iniciando a API ...")

	ctx := context.Background()
	shutdown, err := initTracer(ctx)
	if err != nil {
		log.Fatalf("Erro ao inicializar o OpenTelemetry: %v", err)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Printf("Erro ao encerrar o TracerProvider: %v", err)
		}
	}()

	jokeFactorURL = os.Getenv("JOKE_FACTOR_URL")
	if jokeFactorURL == "" {
		jokeFactorURL = "http://localhost:8000/joke"
		log.Println("JOKE_FACTOR_URL não definido, usando padrão:", jokeFactorURL)
	}

	initDB()
	defer db.Close()

	r := chi.NewRouter()
	r.Get("/backend", getHandler)
	r.Post("/backend", postHandler)
	r.Handle("/metrics", promhttp.Handler())

	log.Println("Servidor rodando na porta 7000 ...")
	log.Fatal(http.ListenAndServe(":7000", r))
}
