FROM golang:1.26-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG APP=api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${APP}

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/app /app/app
USER 65532:65532
ENTRYPOINT ["/app/app"]
