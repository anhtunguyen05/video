package rabbitmq

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestConcurrentDeadOutboxClaimsAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(6)
	ctx := context.Background()
	ids := make(map[string]bool)
	for i := 0; i < 3; i++ {
		id := uuid.NewString()
		if _, err := db.ExecContext(ctx, `INSERT INTO outbox_events
			(id, event_key, exchange_name, routing_key, payload)
			VALUES ($1, $2, 'processing.dead', 'processing.dead.v1', '{}'::jsonb)`, id, id+":dead"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE id = $1`, id) })
		ids[id] = true
	}
	type result struct {
		event deadEvent
		ok    bool
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 3)
	for i := 0; i < 3; i++ {
		go func() {
			<-start
			event, ok, err := claimDeadEvent(ctx, db)
			results <- result{event, ok, err}
		}()
	}
	close(start)
	seen := make(map[string]bool)
	for i := 0; i < 3; i++ {
		claim := <-results
		if claim.err != nil || !claim.ok || !ids[claim.event.id] || seen[claim.event.id] {
			t.Fatalf("invalid dead-outbox claim: %+v", claim)
		}
		seen[claim.event.id] = true
	}
	if _, ok, err := claimDeadEvent(ctx, db); err != nil || ok {
		t.Fatalf("fourth publisher claimed a locked event: ok=%v err=%v", ok, err)
	}
}
