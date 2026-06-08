FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY . .
RUN go build -o /out/fake-ollama ./test/e2e/fakes/ollama

FROM alpine:3.19

RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/fake-ollama .
ENTRYPOINT ["./fake-ollama"]
