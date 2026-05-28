# Stage 1: Build Go binary
FROM golang:1.26-alpine AS go-builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o phosche ./cmd/phosche/

# Stage 2: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 3: Runtime
FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /app/phosche .
COPY --from=frontend-builder /app/web/dist ./web/dist
COPY config.example.yaml ./config.yaml
EXPOSE 8080
ENTRYPOINT ["./phosche"]
CMD ["-config", "config.yaml"]
