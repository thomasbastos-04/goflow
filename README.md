# GoFlow

Projeto backend em Go com nível intermediário/pleno para demonstrar arquitetura, regras de negócio, mensageria, concorrência, idempotência e Docker.

## Arquitetura

- API HTTP em Go (stdlib `net/http`)
- PostgreSQL
- Redis para idempotência
- RabbitMQ para eventos
- Worker assíncrono de pedidos
- Transactional Outbox
- JWT
- Logs estruturados com Zap
- Graceful shutdown
- Health/readiness endpoints
- Testes unitários
- Docker Compose

Fluxo principal:

`POST /api/v1/orders` -> transação PostgreSQL -> `orders + outbox_events` -> Outbox Publisher -> RabbitMQ -> Worker -> `PAID` ou `FAILED`.

## Rodando

```bash
cp .env.example .env
docker compose up --build
```

API: `http://localhost:8080`  
RabbitMQ Management: `http://localhost:15672` (`guest` / `guest`)

### 1. Registrar

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Thomas","email":"thomas@example.com","password":"12345678"}'
```

Copie o `token`.

### 2. Criar produto

```bash
curl -X POST http://localhost:8080/api/v1/products \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Mechanical Keyboard","sku":"KB-001","price_cents":34990,"stock":10}'
```

### 3. Criar pedido

```bash
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Idempotency-Key: pedido-001" \
  -H "Content-Type: application/json" \
  -d '{"items":[{"product_id":"ID_DO_PRODUTO","quantity":2}]}'
```

### Endpoints

- `GET /health`
- `GET /ready`
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `GET /api/v1/products`
- `POST /api/v1/products`
- `POST /api/v1/orders`
- `GET /api/v1/orders`
- `GET /api/v1/orders/{id}`

## Decisões técnicas

**Outbox:** pedido e evento são persistidos na mesma transação. O publisher envia eventos pendentes ao RabbitMQ.

**Idempotência:** `Idempotency-Key` é registrada no Redis para evitar criação duplicada de pedidos.

**Concorrência de estoque:** o serviço usa `SELECT ... FOR UPDATE` dentro da transação de criação do pedido.

**Worker:** consome `order.created`, simula processamento de pagamento e atualiza o estado do pedido.

**DLQ:** mensagens rejeitadas após falha de processamento são roteadas para `orders.dlq`.

> O pagamento é propositalmente simulado: pedidos cujo total em centavos é divisível por 13 falham, permitindo testar o fluxo de erro de forma determinística.

## Testes

```bash
go test ./... -race -cover
```

## V2 - Observabilidade com Elastic Stack

A V2 adiciona **Elasticsearch + Kibana + Filebeat** sem alterar o PostgreSQL como fonte oficial dos dados de negócio.

### Componentes

- `elasticsearch`: armazena e indexa logs.
- `kibana`: interface para pesquisa e visualização.
- `filebeat`: descobre os containers Go via Docker labels e envia seus logs JSON para o Elasticsearch.
- `api`, `worker` e `outbox`: continuam escrevendo logs estruturados com Zap no stdout.

### Subir a V2

```bash
docker compose down
docker compose up --build
```

Serviços:

- API: http://localhost:8080
- RabbitMQ: http://localhost:15672
- Elasticsearch: http://localhost:9200
- Kibana: http://localhost:5601

> Elasticsearch/Kibana podem levar mais tempo para iniciar na primeira execução. Reserve pelo menos ~2 GB de memória livre para a stack Docker completa.

### Validar Elasticsearch

```bash
curl http://localhost:9200/_cluster/health?pretty
```

### Visualizar logs no Kibana

1. Abra `http://localhost:5601`.
2. Vá em **Discover**.
3. Crie/seleciona um Data View para `filebeat-*` caso ele ainda não apareça automaticamente.
4. Pesquise por `service.name : "goflow-api"`, `service.name : "goflow-worker"` ou `service.name : "goflow-outbox"`.

Depois de criar um pedido, procure por `msg : "event_published"` e `msg : "order_processed"` para acompanhar o fluxo assíncrono.

### Arquitetura V2

```text
Client -> Go API -> PostgreSQL -> Outbox -> RabbitMQ -> Worker
             |                         |                  |
             +----------- JSON logs --+------------------+
                                      |
                                   Filebeat
                                      |
                               Elasticsearch
                                      |
                                    Kibana
```

### Identidade do módulo

O module path foi atualizado para:

```text
github.com/thomasbastos-04/goflow
```

