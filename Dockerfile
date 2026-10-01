FROM golang:1.26.2

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

# Copy the source code
COPY . .

RUN go build -o main ./cmd/ratelimiter

# Expose the application port
EXPOSE 10000

CMD ["./main"]