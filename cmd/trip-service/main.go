package main

import (
	"context"
	"log"

	"template/internal/database"
	"template/internal/handler"
	"template/internal/server"
	"template/internal/validate"
)

func main() {
	cfg, err := validate.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx := context.Background()

	dbPool, err := database.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer dbPool.Close()

	apiHandler := handler.NewAPIHandler(dbPool)
	srv := server.NewServer(cfg, apiHandler)

	if err := srv.Run(); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
