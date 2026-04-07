.PHONY: deps build clean test cli docker-build docker-run compose-up compose-down

IMAGE ?= slimebot:latest
SLIMEBOT_HOME ?= $(HOME)/.slimebot
PORT ?= 8080
SHELL := /bin/sh

deps:
	npm install
	npm install --prefix frontend

build:
	npm run build:frontend
	go build -o slimebot ./cmd/server

cli:
	npm --prefix cli install
	npm --prefix cli run build
	go build -o slimebot-cli ./cmd/cli

clean:
	$(RM) -f slimebot slimebot.exe slimebot-cli slimebot-cli.exe
	@if [ -d web/dist ]; then find web/dist -mindepth 1 -delete; fi
	rm -rf cli/dist

test:
	go test ./...

docker-build:
	docker build -t $(IMAGE) .

docker-run:
	docker run --rm \
		-p $(PORT):8080 \
		-v "$(SLIMEBOT_HOME):/home/slimebot/.slimebot" \
		$(IMAGE)

compose-up:
	docker compose up -d

compose-down:
	docker compose down
