FROM node:22-bookworm AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./frontend/
RUN npm ci --prefix frontend
COPY frontend ./frontend
RUN npm run build --prefix frontend

FROM golang:1.24-bookworm AS go-builder
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev && rm -rf /var/lib/apt/lists/*
ENV CGO_ENABLED=1 GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
COPY --from=frontend /src/web/dist ./web/dist
RUN go build -trimpath -ldflags="-s -w" -o /slimebot ./cmd/server

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates python3 python3-pip python3-venv \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/python3 /usr/bin/python
ENV EMBEDDING_PYTHON_BIN=/usr/bin/python3
WORKDIR /app
COPY requirements.txt .
RUN pip3 install --no-cache-dir --break-system-packages -r requirements.txt
COPY --from=go-builder /slimebot /app/slimebot
COPY scripts ./scripts
RUN groupadd -g 1000 slimebot && \
    useradd -u 1000 -g 1000 -m slimebot && \
    mkdir -p /app/storage && \
    chown -R slimebot:slimebot /app
USER slimebot
EXPOSE 8080
ENTRYPOINT ["/app/slimebot"]
