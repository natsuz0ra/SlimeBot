.PHONY: deps build clean test docker-build docker-run compose-up compose-down

IMAGE ?= slimebot:latest
SHELL := /bin/sh

deps:
	npm install
	npm install --prefix frontend

build:
	npm run build

clean:
	$(RM) -f slimebot slimebot.exe
	@if [ -d web/dist ]; then find web/dist -mindepth 1 -delete; fi

test:
	go test ./...

docker-build:
	docker build -t $(IMAGE) .

docker-run:
	docker run --rm -p 8080:8080 \
		-v "$(CURDIR)/storage:/app/storage" \
		-v "$(CURDIR)/onnx:/app/onnx" \
		--env-file .env \
		$(IMAGE)

compose-up:
	docker compose up -d

compose-down:
	docker compose down
