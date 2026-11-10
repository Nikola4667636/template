-include .env
export

TARGET_FILE=./cmd/trip-service
API_PATH=./internal/generated/api.gen.go


.PHONY: test test-integration run migrate lint generate env-start env-stop

env-start:
	tripgoctl environment start

env-stop:
	tripgoctl environment stop

test:
	go test -race -count=2 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

test-integration:
	go test -race -count=2 -tags=integration ./internal/repository/... ./internal/infra/...

run:
	go run "$(TARGET_FILE)"

migrate:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" up

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-package api \
		-o "$(API_PATH)" \
		contracts/openapi/trip-service.openapi.yaml
		go generate ./...

lint:
	golangci-lint run ./...
