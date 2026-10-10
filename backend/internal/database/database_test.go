package database

import (
	"context"
	"os"
	"testing"
	"time"

	"website/backend/internal/config"
)

func testConfig(url string) config.Config {
	return config.Config{
		DatabaseURL:       url,
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 30 * time.Minute,
	}
}

func TestOpenRejectsUnreachableDatabase(t *testing.T) {
	_, err := Open(context.Background(), testConfig("postgresql://nobody:nobody@127.0.0.1:1/nowhere?sslmode=disable"))
	if err == nil {
		t.Fatal("expected error for unreachable database")
	}
}

func TestOpenPingClose(t *testing.T) {
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("SKIP: DATABASE_URL_TEST is not set")
	}
	db, err := Open(context.Background(), testConfig(url))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := db.Ping(context.Background()); err == nil {
		t.Fatal("expected ping to fail after close")
	}
}
