// Package main implementa a API JokeFactor, responsável por buscar piadas do
// Chuck Norris na API pública https://api.chucknorris.io/jokes/random, persistir
// os dados retornados em um banco de dados MySQL e devolver o texto da piada ao
// chamador.
//
// A aplicação é instrumentada com OpenTelemetry, propagando o contexto de trace
// recebido via header W3C TraceContext (traceparent) e criando spans filhos para
// rastrear a chamada HTTP externa e a gravação no banco de dados.
//
// A API expõe os seguintes endpoints:
//   - GET /joke    — busca uma piada aleatória, persiste e retorna o texto
//   - GET /metrics — expõe métricas no formato Prometheus
//
// As seguintes variáveis de ambiente devem ser configuradas:
//   - DB_USER                      — usuário do banco de dados
//   - DB_PASSWORD                  — senha do banco de dados
//   - DB_HOST                      — host (e porta) do banco de dados (ex: localhost:3306)
//   - DB_NAME                      — nome do banco de dados
//   - OTEL_EXPORTER_OTLP_ENDPOINT  — endereço do OpenTelemetry Collector (ex: otel-collector:4317)
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

const chuckNorrisAPI = "https://api.chucknorris.io/jokes/random"

// ChuckNorrisJoke representa o subconjunto de campos retornados pela API pública
// que são relevantes para esta aplicação.
type ChuckNorrisJoke struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	Value     string `json:"value"`
}

// JokeResponse é o corpo da resposta retornado pelo endpoint GET /joke.
type JokeResponse struct {
	Value string `json:"value"`
}

var db *sql.DB
var tracer trace.Tracer

var httpClient = &http.Client{Timeout: 3 * time.Second}

// dbName e dbHost armazenam as coordenadas do banco para uso nos atributos dos spans.
var dbName, dbHost string

var (
	totalGets = promauto.NewCounter(prometheus.CounterOpts{
		Name: "joke_factor_total_gets",
		Help: "Numero total de requisições GET processadas.",
	})
	totalErros = promauto.NewCounter(prometheus.CounterOpts{
		Name: "joke_factor_total_errors",
		Help: "Numero total de erros processados",
	})
)

// initTracer configura e inicializa o TracerProvider do OpenTelemetry, exportando
// traces para o OpenTelemetry Collector via gRPC. O endereço do collector é lido
// da variável de ambiente OTEL_EXPORTER_OTLP_ENDPOINT.
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
			semconv.ServiceName("joke-factor"),
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
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = otel.Tracer("joke-factor")

	log.Printf("OpenTelemetry inicializado, enviando traces para %s", collectorAddr)
	return tp.Shutdown, nil
}

// initDB inicializa a conexão com o banco de dados MySQL e garante que a tabela
// "piadas" existe, criando-a se necessário.
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

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS piadas (
		id VARCHAR(50) PRIMARY KEY,
		created_at DATETIME NOT NULL,
		value TEXT NOT NULL
	)`)
	if err != nil {
		log.Fatalf("Erro ao criar tabela piadas: %v", err)
	}

	log.Println("Tabela piadas verificada/criada com sucesso")
}

// fetchJoke realiza uma requisição HTTP GET à API pública do Chuck Norris dentro
// de um span OpenTelemetry (KIND=CLIENT). Retorna a piada preenchida ou erro.
func fetchJoke(ctx context.Context) (*ChuckNorrisJoke, error) {
	ctx, span := tracer.Start(ctx, "http.get_chuck_norris_joke",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.method", "GET"),
			attribute.String("http.url", chuckNorrisAPI),
			attribute.String("server.address", "api.chucknorris.io"),
			attribute.Int("server.port", 443),
			attribute.String("network.protocol.name", "https"),
			attribute.String("peer.service", "api.chucknorris.io"),
		),
	)
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chuckNorrisAPI, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("erro ao criar requisição HTTP: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("erro ao chamar api.chucknorris.io: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("api.chucknorris.io retornou status %d", resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var joke ChuckNorrisJoke
	if err := json.NewDecoder(resp.Body).Decode(&joke); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("erro ao decodificar resposta da API: %w", err)
	}

	span.SetAttributes(
		attribute.String("chuck_norris.joke_id", joke.ID),
		attribute.String("chuck_norris.value", joke.Value),
	)
	span.SetStatus(codes.Ok, "")

	return &joke, nil
}

// saveJoke persiste a piada recebida na tabela "piadas" dentro de um span
// OpenTelemetry (KIND=CLIENT). Em caso de ID duplicado, a operação é ignorada
// silenciosamente (INSERT IGNORE).
func saveJoke(ctx context.Context, joke *ChuckNorrisJoke) error {
	const insertPiadas = "INSERT IGNORE INTO piadas (id, created_at, value) VALUES (?, ?, ?)"

	ctx, span := tracer.Start(ctx, "db.inserir_piada",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "mysql"),
			attribute.String("db.name", dbName),
			attribute.String("db.server.address", dbHost),
			attribute.String("db.operation", "INSERT"),
			attribute.String("db.sql.table", "piadas"),
			attribute.String("db.statement", insertPiadas),
			attribute.String("chuck_norris.joke_id", joke.ID),
		),
	)
	defer span.End()

	createdAt, err := time.Parse("2006-01-02 15:04:05.999999", joke.CreatedAt)
	if err != nil {
		// Fallback: usa o timestamp atual se o formato da API mudar.
		createdAt = time.Now()
		log.Printf("Aviso: não foi possível parsear created_at '%s', usando time.Now()", joke.CreatedAt)
	}

	_, err = db.ExecContext(ctx, insertPiadas,
		joke.ID, createdAt, joke.Value,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("erro ao inserir piada no banco: %w", err)
	}

	span.SetStatus(codes.Ok, "")
	log.Printf("Piada %s persistida com sucesso", joke.ID)
	return nil
}

// healthHandler trata requisições GET no endpoint /health.
// Serve como health check simples sem efeitos colaterais (sem chamadas externas
// nem acesso ao banco), retornando HTTP 200 com status "ok".
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// jokeHandler trata requisições GET no endpoint /joke.
// Extrai o contexto de trace distribuído dos headers HTTP recebidos (traceparent),
// busca uma piada na API pública, persiste no banco e retorna o texto em JSON.
func jokeHandler(w http.ResponseWriter, r *http.Request) {
	totalGets.Inc()
	log.Println("Requisição GET recebida em /joke")

	// Extrai o contexto de trace distribuído dos headers HTTP (header "traceparent").
	// Os spans criados a seguir serão filhos do trace iniciado pelo go-calculator.
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

	joke, err := fetchJoke(ctx)
	if err != nil {
		totalErros.Inc()
		log.Printf("Erro ao buscar piada: %v", err)
		http.Error(w, "Erro ao buscar piada do Chuck Norris", http.StatusInternalServerError)
		return
	}

	if err := saveJoke(ctx, joke); err != nil {
		totalErros.Inc()
		log.Printf("Erro ao salvar piada: %v", err)
		http.Error(w, "Erro ao salvar piada no banco de dados", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(JokeResponse{Value: joke.Value})
}

// main é o ponto de entrada da aplicação. Inicializa o OpenTelemetry, o banco de
// dados, configura o roteador HTTP e inicia o servidor na porta 8000.
func main() {
	log.Println("Iniciando o JokeFactor ...")

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

	initDB()
	defer db.Close()

	r := chi.NewRouter()
	r.Get("/health", healthHandler)
	r.Get("/joke", jokeHandler)
	r.Handle("/metrics", promhttp.Handler())

	log.Println("Servidor rodando na porta 8000 ...")
	log.Fatal(http.ListenAndServe(":8000", r))
}
