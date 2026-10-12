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

func TestOpenSurvivesUnreachableDatabase(t *testing.T) {
	db, err := Open(context.Background(), testConfig("postgresql://nobody:nobody@127.0.0.1:1/nowhere?sslmode=disable"))
	if err != nil {
		t.Fatalf("open must not fail when the database is unreachable: %v", err)
	}
	if db == nil {
		t.Fatal("expected a usable handle")
	}
	if db.Client == nil {
		t.Fatal("expected the ent client to be built even when unreachable")
	}
	if db.StartupError() == nil {
		t.Fatal("expected the failed probe to be recorded as a startup error")
	}
	if err := db.Ping(context.Background()); err == nil {
		t.Fatal("expected ping to keep failing while the database is down")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestOpenRecordsUnparsableDatabaseURL(t *testing.T) {
	db, err := Open(context.Background(), testConfig("not-a-dsn-at-all"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if db.StartupError() == nil {
		t.Fatal("expected an unparsable database url to be recorded as a startup error")
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
	if db.StartupError() != nil {
		t.Fatalf("startup error = %v, want nil for a reachable database", db.StartupError())
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := db.Ping(context.Background()); err == nil {
		t.Fatal("expected ping to fail after close")
	}
}

func TestOpenBuildsEntClient(t *testing.T) {
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("SKIP: DATABASE_URL_TEST is not set")
	}
	db, err := Open(context.Background(), testConfig(url))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if db.Client == nil {
		t.Fatal("expected ent client to be built")
	}
	if err := db.SQL().PingContext(context.Background()); err != nil {
		t.Fatalf("sql ping: %v", err)
	}
}
