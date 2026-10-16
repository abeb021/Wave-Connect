FROM golang:alpine3.23
WORKDIR /app

COPY pkg/ ./pkg/
COPY chat-service/ ./chat-service/

WORKDIR /app/chat-service
RUN go mod download
RUN go build -o main ./cmd
EXPOSE 8082
CMD ["./main"]