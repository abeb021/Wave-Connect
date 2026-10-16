FROM golang:alpine3.23

WORKDIR /app

COPY pkg/ ./pkg/
COPY gateway-service/ ./gateway-service/

WORKDIR /app/gateway-service

RUN go mod download
RUN go build -o main ./cmd

EXPOSE 8080
CMD ["./main"]