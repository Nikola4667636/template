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
	echo "Имитация накатывания миграций" 

generate:
	echo "Имитация генерирования API"

lint:
	echo "Появится реализация в будущем"