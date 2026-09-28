package main

import (
	"log"

	"template/internal/handler"
	"template/internal/server"
	"template/internal/validate"
)

func main() {
	cfg, err := validate.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	healthHandler := handler.NewHealthHandler(nil)

	srv := server.NewServer(cfg, healthHandler)

	if err := srv.Run(); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
