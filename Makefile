.PHONY: build run migrate

build:
	go build -o pr-service ./cmd/service

run: build
	./pr-service

migrate:
	docker-compose run --rm db bash -c "psql -U postgres -d prservice -f /migrations/001_init.sql"

up:
	docker-compose up --build
