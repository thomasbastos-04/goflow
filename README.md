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
