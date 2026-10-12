package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"website/backend/internal/config"
	"website/backend/internal/database/ent"
)

const pingTimeout = 5 * time.Second

type Database struct {
	db         *sql.DB
	Client     *ent.Client
	startupErr error
}

func Open(ctx context.Context, cfg config.Config) (*Database, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	d := &Database{
		db:     db,
		Client: ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db))),
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		d.startupErr = fmt.Errorf("database: connect: %w", err)
	}

	return d, nil
}

func (d *Database) StartupError() error {
	return d.startupErr
}

func (d *Database) SQL() *sql.DB {
	return d.db
}

func (d *Database) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *Database) Close() error {
	if err := d.Client.Close(); err != nil {
		_ = d.db.Close()
		return err
	}
	return d.db.Close()
}
