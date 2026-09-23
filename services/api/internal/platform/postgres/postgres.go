package postgres

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB interface {
	PingContext(context.Context) error
	Close() error
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func Open(databaseURL string) (*sql.DB, error) {
	return sql.Open("pgx", databaseURL)
}
