FROM golang:alpine3.23

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY pkg/ ./pkg/
COPY profile-service/ ./profile-service/

WORKDIR /app/profile-service

RUN go mod download

RUN go build -tags musl -o main ./cmd

EXPOSE 8084

CMD ["./main"]