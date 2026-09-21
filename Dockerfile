# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS backend
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
COPY --from=frontend /src/web/dist ./internal/server/ui/dist
# Avoid `go mod tidy` in-image (QEMU amd64 builds often SIGSEGV the toolchain).
ENV GOTOOLCHAIN=local
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/lens ./cmd/lens

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget \
  && adduser -D -H -u 10001 lens
USER lens
WORKDIR /data
COPY --from=backend /out/lens /usr/local/bin/lens
EXPOSE 8090
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/lens"]
CMD ["serve"]
