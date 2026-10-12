package main

import (
	"context"
	"log"

	"website/backend/internal/app"
	"website/backend/internal/config"
	"website/backend/internal/database"
)

func main() {
	created, err := config.EnsureDotEnv()
	if err != nil {
		log.Printf("dotenv bootstrap: %v", err)
	}
	if created {
		log.Printf("created .env with a freshly generated TOTP_ENCRYPTION_KEY; keep this file, losing it breaks 2FA for accounts that enabled it")
	}

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

	if err := db.StartupError(); err != nil {
		log.Printf("degraded start: %v; serving anyway, database-backed endpoints return 503 until it answers", err)
	}

	application, err := app.New(cfg, db)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("listening on %s", cfg.Addr)
	if err := application.Run(cfg.Addr); err != nil {
		log.Fatal(err)
	}
}
