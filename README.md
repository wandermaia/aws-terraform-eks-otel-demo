# aws-terraform-eks-otel-demo

Demo de observabilidade com OpenTelemetry e SigNoz rodando em Amazon EKS. O projeto provisiona toda a infraestrutura via Terraform e faz o deploy de três aplicações de exemplo que se comunicam entre si e enviam traces distribuídos para o SigNoz APM.

## Índice

- [A aplicação de demonstração](#a-aplicação-de-demonstração)
- [Visão geral da arquitetura](#visão-geral-da-arquitetura)
- [Trace distribuído](#trace-distribuído)
- [Arquitetura de rede](#arquitetura-de-rede)
- [Estrutura do repositório](#estrutura-do-repositório)
- [Infraestrutura (Terraform)](#infraestrutura-terraform)
- [Aplicações](#aplicações)
- [Banco de dados](#banco-de-dados)
- [Pipelines CI/CD (GitHub Actions)](#pipelines-cicd-github-actions)
- [Como executar localmente](#como-executar-localmente)
- [Observabilidade](#observabilidade)
- [Referências](#referências)

## A aplicação de demonstração

A demo simula uma calculadora com personalidade: o usuário informa seu nome e dois inteiros e recebe o resultado da soma acompanhado de uma frase aleatória do Chuck Norris. Por baixo desse fluxo simples, três serviços se comunicam em cadeia, cada um instrumentado com OpenTelemetry, gerando um trace distribuído completo visível no SigNoz.

### Fluxo de uma requisição

```mermaid
sequenceDiagram
    actor Usuário
    participant Front as chuck-norris-calculator<br/>(Flask :5000)
    participant Back as go-calculator<br/>(Go :7000)
    participant Joke as joke-factor<br/>(Go :8000)
    participant Ext as api.chucknorris.io
    participant DB as MySQL RDS
    participant OTEL as OTel Collector → SigNoz

    Usuário->>Front: POST /frontend<br/>(nome, n1, n2)
    Front->>Back: POST /backend<br/>(W3C traceparent)
    Back->>Joke: GET /joke<br/>(W3C traceparent)
    Joke->>Ext: GET /jokes/random (HTTPS)
    Ext-->>Joke: { value: "piada..." }
    Joke->>DB: INSERT INTO piadas
    Joke-->>Back: { joke: "piada..." }
    Back->>DB: INSERT INTO operacoes
    Back-->>Front: { resultado: n1+n2, joke: "..." }
    Front-->>Usuário: Página com resultado e frase

    Front-)OTEL: spans (gRPC 4317)
    Back-)OTEL: spans (gRPC 4317)
    Joke-)OTEL: spans (gRPC 4317)
```

A propagação de contexto usa o header HTTP **W3C TraceContext** (`traceparent`) em todas as chamadas entre serviços, garantindo que todos os spans de uma requisição compartilhem o mesmo `trace_id`.

### O que está instrumentado

| Serviço (`service.name`) | Span | Kind | O que mede |
|---|---|---|---|
| `chuck-norris-calculator` | `api.calcular_soma` | CLIENT | Chamada HTTP POST ao go-calculator |
| `go-calculator` | `http.get_joke` | CLIENT | Chamada HTTP GET ao joke-factor |
| `go-calculator` | `db.inserir_operacao` | CLIENT | INSERT na tabela `operacoes` do MySQL |
| `joke-factor` | `http.get_chuck_norris_joke` | CLIENT | Chamada HTTPS à API pública do Chuck Norris |
| `joke-factor` | `db.inserir_piada` | CLIENT | INSERT na tabela `piadas` do MySQL |

Cada span carrega atributos de contexto (nome do usuário, operandos, resultado) e o `trace_id` propaga pela cadeia inteira, permitindo visualizar a requisição de ponta a ponta no SigNoz como um único trace com cinco spans aninhados.

### Timeouts configurados

| Camada | Timeout | Detalhe |
|---|---|---|
| `magic-calculator` → `go-calculator` | 10 s | `requests.post(..., timeout=10)` |
| `go-calculator` → MySQL (`db.inserir_operacao`) | 5 s | `context.WithTimeout` no `db.ExecContext` |
| `joke-factor` → `api.chucknorris.io` | 3 s | `http.Client{Timeout: 3s}` dedicado |

## Visão geral da arquitetura

```mermaid
graph TD
    subgraph AWS["AWS (us-west-2)"]
        subgraph VPC["VPC (10.45.0.0/16)"]
            subgraph EKS["EKS Auto Mode - eks-demo-open-telemetry"]
                subgraph NS_MAGIC["namespace: magic"]
                    FRONT["chuck-norris-calculator<br/>(frontend Flask)"]
                end
                subgraph NS_CALC["namespace: calculator"]
                    BACK["go-calculator<br/>(backend Go)"]
                    JOKE["joke-factor<br/>(API Go)"]
                end
                subgraph NS_SIGNOZ["namespace: signoz"]
                    SIGNOZ["SigNoz APM"]
                    OTEL["OTel Collector"]
                end
            end
            RDS["RDS MySQL 8.0<br/>(db.t4g.micro)"]
            ECR["ECR<br/>calculadora-backend<br/>calculadora-frontend<br/>joke-factor"]
        end
    end
    EXT["api.chucknorris.io"]

    FRONT -->|"POST /backend"| BACK
    BACK -->|"GET /joke"| JOKE
    JOKE -->|"GET /jokes/random (HTTPS)"| EXT
    BACK -->|"MySQL 3306 — tabela operacoes"| RDS
    JOKE -->|"MySQL 3306 — tabela piadas"| RDS
    FRONT -->|"traces gRPC 4317"| OTEL
    BACK -->|"traces gRPC 4317"| OTEL
    JOKE -->|"traces gRPC 4317"| OTEL
    OTEL --> SIGNOZ
```

## Trace distribuído

Cada requisição gera um trace completo visível no Jaeger/SigNoz com a seguinte hierarquia de spans:

```mermaid
graph TD
    A["chuck-norris-calculator<br/>api.calcular_soma (CLIENT)"]
    A --> B["go-calculator<br/>db.inserir_operacao (CLIENT)"]
    A --> C["go-calculator<br/>http.get_joke (CLIENT)"]
    C --> D["joke-factor<br/>http.get_chuck_norris_joke (CLIENT)"]
    C --> E["joke-factor<br/>db.inserir_piada (CLIENT)"]
```

A propagação usa W3C TraceContext (`traceparent`) em todas as chamadas HTTP entre serviços.

## Arquitetura de rede

A VPC é dividida em 2 AZs, cada uma com três camadas de subnet (pública, privada e de banco de dados), seguindo o padrão de isolamento por camada:

```mermaid
graph TB
    subgraph AWS["AWS us-west-2"]
        IGW["Internet Gateway"]

        subgraph VPC["VPC vpc-eks-demo-open-telemetry (10.45.0.0/16)"]
            subgraph AZA["AZ us-west-2a"]
                PubA["Subnet pública<br/>10.45.48.0/24"]
                PrivA["Subnet privada<br/>10.45.4.0/22"]
                DbA["Subnet database<br/>10.45.0.0/24"]
            end

            subgraph AZB["AZ us-west-2b"]
                PubB["Subnet pública<br/>10.45.49.0/24"]
                PrivB["Subnet privada<br/>10.45.8.0/22"]
                DbB["Subnet database<br/>10.45.1.0/24"]
            end

            NAT["NAT Gateway<br/>(single, na subnet pública AZ-a)"]
            ALBExt["ALB externo<br/>ingressClass: alb-external<br/>(HTTPS, frontend)"]
            ALBInt["ALB interno<br/>ingressClass: alb-internal<br/>(padrão, SigNoz)"]
            EKSNodes["Nós EKS Auto Mode<br/>(Karpenter NodePool)<br/>magic / calculator / signoz"]
            RDS["RDS MySQL 8.0<br/>db.t4g.micro<br/>Multi-AZ subnet group"]
            R53["Route53 private zone<br/>wandermaia.com<br/>CNAME mysql-lab.wandermaia.com"]
        end
    end

    Internet(("Internet"))

    Internet --> IGW
    IGW --> PubA
    IGW --> PubB
    PubA --> NAT
    PubA --> ALBExt
    PubB --> ALBExt
    NAT --> PrivA
    NAT --> PrivB
    ALBExt --> EKSNodes
    ALBInt --> EKSNodes
    PrivA --> EKSNodes
    PrivB --> EKSNodes
    EKSNodes -->|"3306"| RDS
    RDS --- DbA
    RDS --- DbB
    R53 -.->|resolve interno| RDS
```

- **Subnets públicas**: hospedam o NAT Gateway (single, para reduzir custo) e recebem tráfego do ALB externo (`alb-external`) que expõe o frontend via HTTPS.
- **Subnets privadas**: hospedam os nós do EKS Auto Mode provisionados pelo Karpenter; sem IP público, saem para a internet via NAT Gateway (necessário para pull de imagens públicas e chamada à `api.chucknorris.io`).
- **Subnets database**: isoladas, sem rota para a internet; o RDS MySQL só aceita conexões na porta 3306 vindas dos CIDRs das subnets privadas (security group dedicado).
- **Route53 private zone** (`wandermaia.com`): resolve o endpoint do RDS internamente via CNAME, usado pelas aplicações no lugar do endpoint público do RDS.

## Estrutura do repositório

```
.
├── infra/                              # Terraform — provisionamento da infraestrutura
│   ├── env/
│   │   └── lab/
│   │       ├── backend.hcl             # Configuração do backend S3
│   │       ├── terraform.tfvars        # Variáveis do ambiente lab
│   │       └── K8s-Infra-value.yaml    # Values do chart k8s-infra (OTel)
│   ├── modules/
│   │   └── rds/                        # Módulo local — RDS MySQL + Security Group
│   │       ├── main.tf
│   │       ├── variables.tf
│   │       └── outputs.tf
│   ├── Makefile                        # Atalhos para comandos Terraform
│   ├── destroy_config.json             # Controla destroy vs apply nas pipelines
│   └── *.tf                            # Recursos Terraform raiz
│
└── src/                                # Aplicações de exemplo
    ├── docker-compose.yml              # Sobe todo o stack localmente
    ├── otel-collector-config.yaml      # Config do OTel Collector (compose)
    ├── go-calculator/                  # Backend — API Go (porta 7000)
    │   ├── k8s/manifesto.yaml
    │   └── Dockerfile
    ├── joke-factor/                    # API de piadas — Go (porta 8000)
    │   ├── k8s/manifesto.yaml
    │   └── Dockerfile
    └── magic-calculator/               # Frontend — Flask Python (porta 5000)
        ├── k8s/manifesto.yaml
        ├── img/chuck.png
        └── Dockerfile
```

## Infraestrutura (Terraform)

| Recurso | Descrição |
|---|---|
| VPC | CIDR `10.45.0.0/16`, 2 AZs, subnets pública/privada/database |
| NAT Gateway | Single NAT Gateway |
| EKS Auto Mode | Cluster `eks-demo-open-telemetry`, versão 1.35 |
| Karpenter NodePool | Instâncias `t/c/m/r`, `medium` a `4xlarge`, spot + on-demand |
| SigNoz | Instalado via Helm no namespace `signoz` |
| OTel Collector | Chart `k8s-infra` do SigNoz, coleta métricas do cluster |
| RDS MySQL 8.0 | `db.t4g.micro`, nas subnets de database — provisionado via módulo local `modules/rds` |
| ECR | `calculadora-backend`, `calculadora-frontend`, `joke-factor` |
| ACM | Certificado auto-assinado para HTTPS no ALB do SigNoz e do frontend |
| StorageClass | `sc-ebs-gp3-encrypted` como padrão do cluster |
| IngressClass | `alb-internal` (padrão) e `alb-external` |

### Como o Terraform cria a infraestrutura

O código está organizado como um único state raiz (`infra/*.tf`, sem módulos remotos além de VPC/EKS/RDS), com dependências implícitas (referência a outputs) e explícitas (`depends_on`) entre os recursos. O fluxo de criação segue, em linhas gerais, esta ordem:

```mermaid
graph TD
    VPC["module.vpc<br/>VPC + subnets + NAT + IGW"]
    IAM["aws_iam_role.eks_auto_node_role<br/>(eks-iam.tf)"]
    SG["Security Groups<br/>(EKS adicional + MySQL)"]
    EKS["module.eks_cluster<br/>EKS Auto Mode"]
    SC["StorageClass gp3<br/>(default)"]
    IC["IngressClass<br/>alb-internal / alb-external"]
    NP["Karpenter NodePool<br/>(eks-nodepool.tf)"]
    ADDONS["module.eks_blueprints_addons<br/>(metrics-server, etc.)"]
    RDS["aws_db_instance.db_mysql"]
    R53["Route53 private zone + CNAME"]
    ECR["ECR repos<br/>(backend, frontend, joke-factor)"]
    ACM["Certificado auto-assinado (ACM)"]
    SIGNOZ["Helm release SigNoz<br/>+ k8s-infra (OTel Collector)"]

    VPC --> EKS
    IAM --> EKS
    SG --> EKS
    EKS --> SC
    EKS --> NP
    SC --> IC
    NP --> IC
    IC --> ADDONS
    VPC --> RDS
    SG --> RDS
    RDS --> R53
    ADDONS --> SIGNOZ
    IC --> SIGNOZ
    ACM --> IC
    ECR
```

1. **Rede** (`vpc.tf`): cria a VPC, subnets (pública/privada/database), NAT Gateway e Internet Gateway via módulo `terraform-aws-modules/vpc/aws`.
2. **IAM e Security Groups** (`eks-iam.tf`, `eks-security-groups.tf`, `rds-security-groups.tf`): criam a role usada pelos nós do Karpenter e os security groups do cluster e do MySQL, consumidos pelos recursos seguintes.
3. **EKS Auto Mode** (`eks.tf`): sobe o cluster (`terraform-aws-modules/eks/aws`) nas subnets privadas, com `compute_config.enabled = true` e node pool customizado (sem o pool padrão da AWS).
4. **StorageClass, IngressClass e NodePool** (`eks-storage-class.tf`, `eks-ingress-class.tf`, `eks-nodepool.tf`): objetos Kubernetes aplicados via `kubectl_manifest`, que dependem do cluster já existir.
5. **EKS Blueprints Addons** (`eks-blueprints-addons.tf`): instala addons de suporte (ex.: metrics-server), com `depends_on` explícito no `IngressClass` interno.
6. **RDS** (`rds.tf`) e **Route53** (`route53.tf`): o MySQL é criado nas subnets de database usando o security group dedicado; o CNAME privado é criado logo em seguida, apontando para o endpoint do RDS.
7. **ECR** (`ecr.tf`) e **ACM** (`certificado-auto-assinado.tf`): recursos paralelos, sem dependência direta do cluster; o certificado é usado depois nos manifests de Ingress das aplicações.
8. **SigNoz via Helm** (`signoz-helm.tf`): instalado por último, depende do cluster, do addons e do IngressClass estarem prontos para expor o SigNoz via ALB.

O Terraform resolve essa ordem automaticamente pelo grafo de dependências (outputs referenciados + `depends_on`); a lista acima é apenas a leitura lógica do `terraform plan`.

### Comandos Terraform (via Makefile)

```bash
make init ENV=lab       # Inicializar
make ci-validate        # Validar sem backend
make plan ENV=lab       # Planejar
make apply ENV=lab      # Aplicar
make destroy-plan ENV=lab
make destroy ENV=lab
```

### Controle de destroy nas pipelines

O arquivo `infra/destroy_config.json` controla o comportamento da pipeline de infra:

```json
{
    "lab": false,   // false = provisionar | true = destruir
    "prd": false
}
```

## Aplicações

### chuck-norris-calculator (frontend)

Interface web em Python/Flask. O usuário informa seu nome e dois números inteiros; o resultado da soma é exibido acompanhado de uma frase do Chuck Norris.

- **Porta:** 5000
- **Namespace K8s:** `magic`
- **ECR:** `calculadora-frontend`
- **Endpoint de health:** `GET /frontend`
- **Variável de ambiente:** `API_URL` aponta para o go-calculator via service interno do K8s
- **Ingress:** ALB externo com HTTPS (redirect HTTP→443), `ingressClassName: alb-external`

### go-calculator (backend)

API REST em Go que realiza a soma dos operandos, grava o histórico em MySQL e consulta o joke-factor para obter a piada antes de responder.

- **Porta:** 7000
- **Namespace K8s:** `calculator`
- **ECR:** `calculadora-backend`
- **Endpoint de health:** `GET /backend`
- **Variáveis de ambiente:** `DB_*` para o MySQL, `JOKE_FACTOR_URL` para o joke-factor

### joke-factor (API de piadas)

API REST em Go que busca uma piada aleatória em `api.chucknorris.io`, persiste os dados (id, created_at, value) na tabela `piadas` do MySQL e retorna o texto da piada.

- **Porta:** 8000
- **Namespace K8s:** `calculator`
- **ECR:** `joke-factor`
- **Endpoint de health:** `GET /health`
- **Endpoint principal:** `GET /joke`
- **Variáveis de ambiente:** `DB_*` para o MySQL, `OTEL_EXPORTER_OTLP_ENDPOINT`

## Banco de dados

Um único MySQL compartilhado pelos dois serviços Go:

| Tabela | Serviço | Colunas |
|---|---|---|
| `operacoes` | go-calculator | id, nome, operador1, operador2, data_execucao |
| `piadas` | joke-factor | id (PK), created_at, value |

## Pipelines CI/CD (GitHub Actions)

### Infra

| Workflow | Trigger | Ambiente |
|---|---|---|
| `infra-lab.yml` | Push em `develop` com path `infra/**` ou manual | lab (us-west-2) |
| `infra-prd.yml` | Manual (`workflow_dispatch`) | prd (us-east-1) |

O workflow reutilizável `terraform.yml` executa dois jobs em sequência:
1. **validate** - `make ci-validate` (sem credenciais AWS)
2. **deploy** - `make plan` + `make apply` ou `make destroy-plan` + `make destroy`

### Aplicações

| Workflow | Trigger | Ambiente |
|---|---|---|
| `api-lab-calculadora-api.yml` | Push em `lab` com path `src/go-calculator/**` ou manual | lab |
| `api-prd-calculadora-api.yml` | Push em `main` com path `src/go-calculator/**` ou manual | prd |
| `api-lab-joke-factor.yml` | Push em `lab` com path `src/joke-factor/**` ou manual | lab |
| `api-prd-joke-factor.yml` | Push em `main` com path `src/joke-factor/**` ou manual | prd |
| `front-lab-magic-calculator.yml` | Push em `lab` com path `src/magic-calculator/**` ou manual | lab |
| `front-prd-magic-calculator.yml` | Push em `main` com path `src/magic-calculator/**` ou manual | prd |

O workflow reutilizável `ci-cd.yml` executa dois jobs em sequência:
1. **CI** - build da imagem Docker e push para o ECR
2. **CD** - substituição dos placeholders nos manifestos e deploy no EKS via `kubectl apply`

### Secrets necessários no GitHub

| Secret | Descrição |
|---|---|
| `AWS_ACCESS_KEY_ID` | Chave de acesso AWS |
| `AWS_SECRET_ACCESS_KEY` | Chave secreta AWS |
| `DB_USER` | Usuário do banco de dados |
| `DB_PASSWORD` | Senha do banco de dados |
| `DB_HOST` | Endpoint do RDS MySQL |
| `CERTIFICATE_ARN` | ARN do certificado ACM (output do Terraform), que é usado no Ingress do frontend |
| `DOCKERHUB_USERNAME` | Usuário do Docker Hub (evita rate limit de pull anônimo) |
| `DOCKERHUB_TOKEN` | Access Token do Docker Hub (gerado em Account Settings → Security) |

## Como executar localmente

```bash
cd src
docker compose up --build
```

Serviços disponíveis após o boot:

| Serviço | URL |
|---|---|
| Frontend | http://localhost:5000/frontend |
| go-calculator API | http://localhost:7000/backend |
| joke-factor API | http://localhost:8000/joke |
| Jaeger UI (traces) | http://localhost:16686 |
| Métricas go-calculator | http://localhost:7000/metrics |
| Métricas joke-factor | http://localhost:8000/metrics |

## Observabilidade

O SigNoz é acessível via ALB externo após o provisionamento. O OTel Collector coleta:
- Traces das três aplicações via gRPC (`signoz-otel-collector.signoz.svc.cluster.local:4317`)
- Métricas de infraestrutura do cluster EKS (nodes, pods, deployments)
- Logs do cluster

## Referências

### OpenTelemetry

- [W3C TraceContext — especificação do header `traceparent`](https://www.w3.org/TR/trace-context/)
- [OpenTelemetry SDK Go](https://opentelemetry.io/docs/languages/go/)
- [OpenTelemetry SDK Python](https://opentelemetry.io/docs/languages/python/)
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)

### SigNoz

- [Instalação do SigNoz no Kubernetes (Helm)](https://signoz.io/docs/install/kubernetes/)
- [k8s-infra chart - coleta de métricas e logs do cluster](https://signoz.io/docs/tutorial/kubernetes-infra-metrics/)
- [Kubernetes Dashboards](https://signoz.io/docs/dashboards/dashboard-templates/kubernetes-dashboards/)

### AWS / EKS

- [EKS Auto Mode](https://docs.aws.amazon.com/eks/latest/userguide/automode.html)
- [EKS Blueprints Addons (Terraform)](https://aws-ia.github.io/terraform-aws-eks-blueprints-addons/)
- [Karpenter — NodePool e EC2NodeClass](https://karpenter.sh/docs/)
- [AWS Load Balancer Controller](https://kubernetes-sigs.github.io/aws-load-balancer-controller/)

### Terraform

- [Terraform AWS Provider](https://registry.terraform.io/providers/hashicorp/aws/latest/docs)
- [Módulo terraform-aws-eks](https://registry.terraform.io/modules/terraform-aws-modules/eks/aws/latest)
- [Módulo terraform-aws-vpc](https://registry.terraform.io/modules/terraform-aws-modules/vpc/aws/latest)
- [Módulo terraform-aws-rds](https://registry.terraform.io/modules/terraform-aws-modules/rds/aws/latest)

### APIs e bibliotecas

- [Chuck Norris API](https://api.chucknorris.io/)
- [go-chi/chi - router HTTP para Go](https://github.com/go-chi/chi)
- [Flask - framework web Python](https://flask.palletsprojects.com/)
