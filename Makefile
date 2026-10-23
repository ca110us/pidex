.PHONY: up down tools generate migrate seed serve test lint eval-fetch eval eval-appearance eval-mugs

BACKEND := backend

up: ## Start Postgres (with pgvector) in Docker
	docker compose up -d --build

down:
	docker compose down

tools: ## Install code generators pinned to the versions in go.mod
	cd $(BACKEND) && go install google.golang.org/protobuf/cmd/protoc-gen-go@$$(go list -m -f '{{.Version}}' google.golang.org/protobuf)
	cd $(BACKEND) && go install connectrpc.com/connect/cmd/protoc-gen-connect-go@$$(go list -m -f '{{.Version}}' connectrpc.com/connect)

generate: ## Regenerate API and database code
	buf lint
	buf generate
	cd $(BACKEND) && go tool sqlc generate

migrate:
	cd $(BACKEND) && go run ./cmd/pidex migrate up

seed: ## make seed TEAM="My team" EMAIL=you@example.com TEMPLATE=<template key> (run without TEMPLATE to list them)
	cd $(BACKEND) && go run ./cmd/pidex seed -team "$(TEAM)" -email "$(EMAIL)" -template "$(TEMPLATE)"

serve:
	cd $(BACKEND) && go run ./cmd/pidex serve

test: ## Unit tests plus end-to-end tests (needs `make up`)
	cd $(BACKEND) && go test ./...

eval-fetch: ## Download the public eval photos (not committed)
	eval/fetch.sh commons
	eval/fetch.sh appearance 960
	eval/fetch.sh mugs 960 && python3 eval/mugs/make_shots.py

eval: ## Score a vision model on the eval photos: make eval [MODEL=vendor/model]
	cd $(BACKEND) && go run ./cmd/evalvision -labels ../eval/commons/labels.tsv -images ../eval/commons/images \
		-model "$(or $(MODEL),google/gemini-3.1-flash-lite)" -out ../eval/commons/last-run.jsonl

eval-appearance: ## Score filing by appearance on photos of look-alike pets: make eval-appearance [MATCH=vendor/model]
	cd $(BACKEND) && go run ./cmd/evalappearance -labels ../eval/appearance/labels.tsv -images ../eval/appearance/images \
		-match "$(or $(MATCH),openai/gpt-6-luna)" -analysis ../eval/appearance/descriptions.jsonl

eval-mugs: ## Score filing by appearance on mugs, white ones and one maker's among them: make eval-mugs [MATCH=vendor/model]
	cd $(BACKEND) && go run ./cmd/evalappearance -config ../eval/mugs/mugs.yaml -labels ../eval/mugs/labels.tsv -images ../eval/mugs/images \
		-match "$(or $(MATCH),openai/gpt-6-luna)" -analysis ../eval/mugs/descriptions.jsonl

lint:
	buf lint
	cd $(BACKEND) && go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@latest ./...

.PHONY: ios-project ios-test

IOS_SIM ?= iPhone 16

ios-project: ## Generate ios/Pidex.xcodeproj from ios/project.yml
	cd ios && xcodegen generate

ios-test: ios-project ## iOS unit tests; set PIDEX_SERVER and PIDEX_TOKEN to also run live tests (PIDEX_LIVE_DRAFT=1 adds a paid model call)
	cd ios && TEST_RUNNER_PIDEX_SERVER=$(PIDEX_SERVER) TEST_RUNNER_PIDEX_TOKEN=$(PIDEX_TOKEN) TEST_RUNNER_PIDEX_LIVE_DRAFT=$(PIDEX_LIVE_DRAFT) TEST_RUNNER_PIDEX_SEARCH_TOKEN=$(PIDEX_SEARCH_TOKEN) \
		xcodebuild test -project Pidex.xcodeproj -scheme Pidex \
		-destination 'platform=iOS Simulator,name=$(IOS_SIM)' -clonedSourcePackagesDirPath .build/spm
