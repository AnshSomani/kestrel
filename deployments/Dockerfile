FROM golang:1.26-alpine AS builder

WORKDIR /app

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /kestrel ./cmd/server

# --- Runtime stage ---
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

COPY --from=builder /kestrel /kestrel

EXPOSE 8080

CMD ["/kestrel"]
