FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY . .
RUN go build -o /out/fake-telegram ./test/e2e/fakes/telegram

FROM alpine:3.19

RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/fake-telegram .
ENTRYPOINT ["./fake-telegram"]
