TARGET_FILE=./cmd/trip-service

.PHONY: test run migrate lint generate clean env-start env-stop

env-start:
	tripgoctl environment start

env-stop:
	tripgoctl environment stop

test:
	go test -race ./...

run:
	go run $(TARGET_FILE)

migrate:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" up

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

lint:
	echo "Появится реализация в будущем"