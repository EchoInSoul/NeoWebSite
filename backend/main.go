package main

import (
	"context"
	"log"

	"website/backend/internal/app"
	"website/backend/internal/config"
	"website/backend/internal/database"
)

func main() {
	if err := config.LoadDotEnv(); err != nil {
		log.Printf("dotenv: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.Open(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	log.Printf("listening on %s", cfg.Addr)
	if err := app.New(cfg, db).Run(cfg.Addr); err != nil {
		log.Fatal(err)
	}
}
