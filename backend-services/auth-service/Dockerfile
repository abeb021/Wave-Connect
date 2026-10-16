FROM golang:alpine3.23

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY pkg/ ./pkg/
COPY auth-service/ ./auth-service/

WORKDIR /app/auth-service

RUN go mod download

RUN go build -tags musl -o main ./cmd

EXPOSE 8081

CMD ["./main"]