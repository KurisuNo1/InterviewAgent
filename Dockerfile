# Stage 1: Build
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o interview-server cmd/server/main.go

# Stage 2: Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata nodejs npm
ENV TZ=Asia/Shanghai

WORKDIR /app

COPY --from=builder /build/interview-server .
COPY config/config.yaml ./config/config.yaml
COPY --from=builder /build/web ./web

RUN mkdir -p /app/data /app/logs

EXPOSE 8080

CMD ["./interview-server", "config/config.yaml"]
