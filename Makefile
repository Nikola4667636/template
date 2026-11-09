-include .env
export

TARGET_FILE=./cmd/trip-service
API_PATH=./internal/generated/api.gen.go


.PHONY: test run migrate lint generate clean env-start env-stop

env-start:
	tripgoctl environment start

env-stop:
	tripgoctl environment stop

test:
	go test -race -count=2 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

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