FROM golang:1.26.0 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/api-server ./cmd/api-server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/indexer ./cmd/indexer
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/keeper ./cmd/keeper

FROM gcr.io/distroless/base-debian12

WORKDIR /app

COPY --from=builder /out/ /app/bin/
COPY config /app/config

CMD ["/app/bin/api-server"]
