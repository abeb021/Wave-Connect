FROM golang:alpine3.23

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY pkg/ ./pkg/
COPY feed-service/ ./feed-service/

WORKDIR /app/feed-service

RUN go mod download

RUN go build -tags musl -o main ./cmd

EXPOSE 8083

CMD ["./main"]