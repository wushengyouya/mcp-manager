FROM golang:1.26-alpine AS builder
WORKDIR /app
ARG GOPROXY=https://proxy.golang.org|https://goproxy.cn|direct
ENV GOPROXY=$GOPROXY
COPY go.mod go.sum ./
RUN apk add --no-cache gcc musl-dev
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o /out/mcp-manager ./cmd/server

FROM alpine:3.22
RUN addgroup -S app && adduser -S app -G app && apk add --no-cache wget
WORKDIR /app
COPY --from=builder /out/mcp-manager /app/mcp-manager
COPY deployments/docker/config.prod.yaml /app/config.yaml
RUN mkdir -p /app/data && chown -R app:app /app
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://127.0.0.1:8080/health || exit 1
CMD ["/app/mcp-manager"]
