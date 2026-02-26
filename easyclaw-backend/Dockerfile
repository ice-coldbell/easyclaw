ARG GOLANG_VERSION=1.26

FROM golang:${GOLANG_VERSION}-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download && go mod verify
RUN go build -o ./api-server ./cmd/api-server
RUN go build -o ./indexer ./cmd/indexer
RUN go build -o ./keeper ./cmd/keeper

ARG RELEASE
ENV RELEASE=${RELEASE}

ARG GIT_SHA1
ENV GIT_SHA1=${GIT_SHA1}

ARG GIT_REVISION
ENV GIT_REVISION=${GIT_REVISION}

FROM alpine
WORKDIR /app

RUN apk update
RUN apk add --no-cache tzdata curl

RUN cp /usr/share/zoneinfo/UTC /etc/localtime
RUN echo "UTC" > /etc/timezone

COPY --from=builder /app/api-server /app/api-server
COPY --from=builder /app/indexer /app/indexer
COPY --from=builder /app/keeper /app/keeper

COPY ./entrypoint.sh /app/
ENTRYPOINT ["sh", "/app/entrypoint.sh"]
