FROM node:22-bookworm AS frontend-build
WORKDIR /src

COPY frontend/package.json frontend/package-lock.json ./frontend/
RUN npm ci --prefix frontend

COPY frontend ./frontend
RUN npm run build --prefix frontend

FROM golang:1.26-bookworm AS go-build
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev && rm -rf /var/lib/apt/lists/*
ENV CGO_ENABLED=1 GOTOOLCHAIN=auto
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY prompts ./prompts
COPY web ./web
COPY --from=frontend-build /src/web/dist ./web/dist

RUN go build -trimpath -ldflags="-s -w" -o /out/slimebot ./cmd/server

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

RUN groupadd -g 1000 slimebot && useradd -u 1000 -g 1000 -m slimebot
RUN mkdir -p /home/slimebot/.slimebot/storage /home/slimebot/.slimebot/onnx /home/slimebot/.slimebot/skills

WORKDIR /app
COPY --from=go-build /out/slimebot /app/slimebot
RUN chown -R slimebot:slimebot /app /home/slimebot

ENV HOME=/home/slimebot
USER slimebot

EXPOSE 8080
ENTRYPOINT ["/app/slimebot"]
