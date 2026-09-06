"""
Frontend web da Magic Calculator, implementado com Flask.

Esta aplicação serve como interface gráfica para a API go-calculator.
O usuário preenche um formulário com seu nome e dois operandos inteiros;
o frontend encaminha os dados para a API via HTTP POST e exibe o resultado
da soma ou uma mensagem de erro amigável.

A aplicação é instrumentada com OpenTelemetry, gerando spans para rastrear
as chamadas realizadas à API go-calculator.

Variáveis de ambiente:
    API_URL:
        Endereço completo da API go-calculator
        (padrão: http://localhost:7000/backend)
    OTEL_EXPORTER_OTLP_ENDPOINT:
        Endereço do OpenTelemetry Collector para envio de traces via gRPC
        (padrão: localhost:4317).

Rotas disponíveis:
    GET  /frontend        — exibe o formulário de cálculo.
    POST /frontend        — valida os dados, chama a API e exibe o resultado.
    GET  /frontend/voltar — redireciona de volta ao formulário principal.

Templates utilizados (pasta templates/):
    index.html  — formulário de entrada.
    result.html — página de exibição do resultado.
    error.html  — página de exibição de erros de comunicação com a API.
"""

from flask import Flask, render_template, request, jsonify, redirect, url_for, send_from_directory
import requests
import os
import logging

from opentelemetry import trace, propagate
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.sdk.resources import Resource
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.semconv.resource import ResourceAttributes
from opentelemetry.baggage.propagation import W3CBaggagePropagator
from opentelemetry.propagate import set_global_textmap
from opentelemetry.propagators.composite import CompositePropagator
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator

log = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Inicialização do OpenTelemetry
# ---------------------------------------------------------------------------

def init_tracer() -> trace.Tracer:
    """Configura e inicializa o TracerProvider do OpenTelemetry.

    Lê o endereço do OpenTelemetry Collector a partir da variável de ambiente
    ``OTEL_EXPORTER_OTLP_ENDPOINT``. Caso não esteja definida, utiliza
    ``localhost:4317`` como padrão.

    O exportador utiliza gRPC sem TLS (insecure), adequado para comunicação
    interna em ambientes containerizados.

    Returns:
        trace.Tracer: Tracer configurado para o serviço ``magic-calculator``.
    """
    collector_addr = os.environ.get('OTEL_EXPORTER_OTLP_ENDPOINT', 'localhost:4317')

    resource = Resource.create({
        ResourceAttributes.SERVICE_NAME: 'chuck-norris-calculator',
        ResourceAttributes.SERVICE_VERSION: '1.0.0',
    })

    exporter = OTLPSpanExporter(
        endpoint=collector_addr,
        insecure=True,
    )

    provider = TracerProvider(resource=resource)
    provider.add_span_processor(BatchSpanProcessor(exporter))
    trace.set_tracer_provider(provider)

    # Registra os propagadores W3C TraceContext + Baggage globalmente,
    # alinhando com o backend Go que também usa o propagador composto.
    set_global_textmap(CompositePropagator([
        TraceContextTextMapPropagator(),
        W3CBaggagePropagator(),
    ]))

    log.info('OpenTelemetry inicializado, enviando traces para %s', collector_addr)
    return trace.get_tracer('chuck-norris-calculator')


tracer = init_tracer()

# ---------------------------------------------------------------------------
# Aplicação Flask
# ---------------------------------------------------------------------------

app = Flask(__name__)

# Endereço da API de cálculo. Pode ser sobrescrito pela variável de ambiente API_URL.
API_URL = os.environ.get('API_URL', 'http://localhost:7000/backend')


@app.route('/frontend', methods=['GET', 'POST'])
def index():
    """Rota principal do frontend.

    GET:
        Renderiza o formulário de entrada (index.html).

    POST:
        Lê os campos ``nome``, ``operador1`` e ``operador2`` do formulário,
        aplica validações locais e, se válidos, envia os dados para a API
        go-calculator via POST JSON dentro de um span OpenTelemetry.

        Validações realizadas:
            - ``nome`` deve ter pelo menos 3 caracteres.
            - ``operador1`` e ``operador2`` devem ser números inteiros.

        Em caso de sucesso, renderiza ``result.html`` com o resultado da soma.
        Em caso de erro HTTP ou falha de conexão com a API, renderiza
        ``error.html`` com a descrição do erro. O erro também é registrado
        no span via ``record_exception``.

    Returns:
        Response: Página HTML renderizada conforme o fluxo descrito acima.
    """
    if request.method == 'POST':
        nome = request.form['nome']
        operador1 = request.form['operador1']
        operador2 = request.form['operador2']

        # Valida o tamanho mínimo do nome antes de chamar a API
        if len(nome) < 3:
            return render_template('index.html', error='Nome deve ter pelo menos 3 caracteres.')

        # Garante que os operandos são inteiros válidos.
        try:
            operador1 = int(operador1)
            operador2 = int(operador2)
        except ValueError:
            return render_template('index.html', error='Operadores devem ser números inteiros!')

        data = {'nome': nome, 'operador1': operador1, 'operador2': operador2}
        headers = {'Content-Type': 'application/json'}

        # Span que rastreia a chamada HTTP para a API go-calculator (KIND=CLIENT
        # para que o SigNoz classifique corretamente como chamada de saída).
        with tracer.start_as_current_span(
            'api.calcular_soma',
            kind=trace.SpanKind.CLIENT,
        ) as span:
            span.set_attribute('http.method', 'POST')
            span.set_attribute('http.url', API_URL)
            span.set_attribute('usuario', nome)
            span.set_attribute('operador1', operador1)
            span.set_attribute('operador2', operador2)

            # Injeta o contexto do span atual nos headers HTTP (W3C TraceContext).
            # O header "traceparent" será lido pela API para continuar o mesmo trace.
            propagate.inject(headers)

            try:
                response = requests.post(API_URL, json=data, headers=headers, timeout=10)
                response.raise_for_status()  # Lança exceção para status HTTP de erro (4xx, 5xx).
                span.set_attribute('http.status_code', response.status_code)
                payload = response.json()
                result = payload.get('resultado')
                piada = payload.get('piada', '')
                span.set_attribute('resultado', result)
                return render_template('result.html', result=result, piada=piada)
            except requests.exceptions.RequestException as e:
                # Registra o erro no span e exibe página de erro ao usuário.
                span.record_exception(e)
                span.set_status(trace.StatusCode.ERROR, str(e))
                return render_template('error.html', error=str(e))

    return render_template('index.html')


@app.route('/frontend/img/<path:filename>')
def img(filename):
    return send_from_directory(os.path.join(os.path.dirname(__file__), 'img'), filename)


@app.route('/frontend/voltar')
def voltar():
    """Redireciona o usuário de volta ao formulário principal (/frontend).

    Returns:
        Response: Redirecionamento HTTP 302 para a rota ``index``.
    """
    return redirect(url_for('index'))


if __name__ == '__main__':
    app.run(debug=True, host='0.0.0.0')
