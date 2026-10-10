# ---------- Stage 1: build ----------
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache protobuf protobuf-dev git
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
ENV PATH="/go/bin:${PATH}"

WORKDIR /src
COPY go.mod ./
COPY proto ./proto
COPY gateway ./gateway

RUN mkdir -p gen/itempb gen/claimpb && \
    protoc --proto_path=proto \
      --go_out=. --go_opt=module=lostfound \
      --go-grpc_out=. --go-grpc_opt=module=lostfound \
      proto/item.proto proto/claim.proto

RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./gateway

# ---------- Stage 2: small runtime image ----------
FROM alpine:3.20
RUN adduser -D -u 10001 appuser
COPY --from=builder /out/gateway /usr/local/bin/gateway
USER appuser
EXPOSE 8081
ENTRYPOINT ["gateway"]
