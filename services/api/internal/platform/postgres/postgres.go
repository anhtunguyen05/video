package postgres

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB interface {
	PingContext(context.Context) error
	Close() error
}

func Open(databaseURL string) (*sql.DB, error) {
	return sql.Open("pgx", databaseURL)
}
