package main

import (
	"log"

	"website/backend/internal/app"
	"website/backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("listening on %s", cfg.Addr)
	if err := app.New(cfg).Run(cfg.Addr); err != nil {
		log.Fatal(err)
	}
}
