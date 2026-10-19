package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

func (repository *Repository) ClaimNext(ctx context.Context, now time.Time) (domain.Job, bool, error) {
	runID := uuid.NewString()
	row := repository.db.QueryRowContext(ctx, `
		WITH claimed AS (
			UPDATE processing_jobs j SET status = 'RUNNING', stage = 'VALIDATE_SOURCE',
				attempt = attempt + 1, run_id = $2, lease_until = $1 + interval '90 seconds',
				started_at = $1, updated_at = $1, next_retry_at = NULL,
				error_code = NULL, error_message = NULL
			WHERE j.id = (
				SELECT due.id FROM processing_jobs due JOIN videos v ON v.id = due.video_id
				WHERE due.status = 'QUEUED' AND due.attempt < due.max_attempts
				  AND (due.next_retry_at IS NULL OR due.next_retry_at <= $1)
				  AND v.processing_version = due.processing_version
				  AND v.status IN ('UPLOADED', 'QUEUED', 'PROCESSING')
				ORDER BY due.created_at FOR UPDATE OF due SKIP LOCKED LIMIT 1
			)
			RETURNING j.*
		), created_run AS (
			INSERT INTO processing_runs (id, job_id, object_prefix, status, created_at)
			SELECT run_id, id,
				'users/' || owner_id || '/videos/' || video_id || '/processing/v' || processing_version || '/runs/' || run_id || '/',
				'RUNNING', $1 FROM claimed RETURNING id
		)
		SELECT id, video_id, owner_id, source_object_key, processing_version,
			operation_key, status, attempt, max_attempts, run_id,
			COALESCE(original_message, ''::bytea), created_at, updated_at
		FROM claimed WHERE EXISTS (SELECT 1 FROM created_run)
	`, now, runID)
	var job domain.Job
	var status string
	err := row.Scan(&job.ID, &job.VideoID, &job.OwnerID, &job.SourceObjectKey,
		&job.ProcessingVersion, &job.OperationKey, &status, &job.Attempt,
		&job.MaxAttempts, &job.RunID, &job.OriginalMessage, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, false, nil
	}
	if err != nil {
		return domain.Job{}, false, err
	}
	job.Status = domain.Status(status)
	return job, true, nil
}

func (repository *Repository) RenewLease(ctx context.Context, job domain.Job, now time.Time) (bool, error) {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE processing_jobs j SET lease_until = $3 + interval '90 seconds', updated_at = $3
		WHERE j.id = $1 AND j.run_id = $2 AND j.status = 'RUNNING' AND j.lease_until > $3
		  AND EXISTS (SELECT 1 FROM videos v WHERE v.id = j.video_id
		    AND v.processing_version = j.processing_version)
	`, job.ID, job.RunID, now)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (repository *Repository) RecoverExpired(ctx context.Context, now time.Time) error {
	if _, err := repository.db.ExecContext(ctx, `UPDATE processing_jobs j
		SET status = 'FAILED', error_code = 'STALE_PROCESSING_VERSION',
			error_message = 'A newer processing version superseded this job.',
			finished_at = $1, updated_at = $1
		WHERE j.status = 'QUEUED' AND EXISTS
			(SELECT 1 FROM videos v WHERE v.id = j.video_id AND v.processing_version <> j.processing_version)`, now); err != nil {
		return err
	}
	rows, err := repository.db.QueryContext(ctx, `
		SELECT id::text, run_id::text, attempt, max_attempts
		FROM processing_jobs WHERE status = 'RUNNING' AND lease_until <= $1
	`, now)
	if err != nil {
		return err
	}
	var expired []domain.Job
	for rows.Next() {
		var job domain.Job
		if err := rows.Scan(&job.ID, &job.RunID, &job.Attempt, &job.MaxAttempts); err != nil {
			_ = rows.Close()
			return err
		}
		expired = append(expired, job)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, job := range expired {
		if err := repository.RetryOrFail(ctx, job, "PROCESS_INTERRUPTED", "The processing lease expired.", now); err != nil {
			return err
		}
	}
	return nil
}

func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 30 * time.Second
	case 2:
		return 2 * time.Minute
	default:
		return 10 * time.Minute
	}
}

func (repository *Repository) RetryOrFail(ctx context.Context, job domain.Job, code, message string, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attempt, maxAttempts int
	var currentVersion bool
	var original []byte
	err = tx.QueryRowContext(ctx, `SELECT j.attempt, j.max_attempts,
		COALESCE(j.original_message, ''::bytea), v.processing_version = j.processing_version
		FROM processing_jobs j JOIN videos v ON v.id = j.video_id
		WHERE j.id = $1 AND j.run_id = $2 AND j.status = 'RUNNING' FOR UPDATE OF j`,
		job.ID, job.RunID).Scan(&attempt, &maxAttempts, &original, &currentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE processing_runs SET status = 'FAILED', finished_at = $2
		WHERE id = $1 AND status = 'RUNNING'`, job.RunID, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE processing_operations SET status = 'FAILED', updated_at = $2
		WHERE run_id = $1 AND status = 'SUCCEEDED'`, job.RunID, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM assets a USING processing_runs r, processing_jobs j
		WHERE r.id = $1 AND j.id = r.job_id AND a.video_id = j.video_id
		  AND a.object_key LIKE r.object_prefix || '%'`, job.RunID); err != nil {
		return err
	}
	if !currentVersion {
		_, err = tx.ExecContext(ctx, `UPDATE processing_jobs SET status = 'FAILED', lease_until = NULL,
			finished_at = $2, error_code = 'STALE_PROCESSING_VERSION',
			error_message = 'A newer processing version superseded this job.', updated_at = $2
			WHERE id = $1 AND run_id = $3 AND status = 'RUNNING'`, job.ID, now, job.RunID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	if attempt < maxAttempts {
		_, err = tx.ExecContext(ctx, `UPDATE processing_jobs SET status = 'QUEUED', run_id = NULL,
			lease_until = NULL, next_retry_at = $2, error_code = $3, error_message = $4, updated_at = $5
			WHERE id = $1 AND run_id = $6 AND status = 'RUNNING'`,
			job.ID, now.Add(retryDelay(attempt)), code, message, now, job.RunID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	result, err := tx.ExecContext(ctx, `
		WITH failed_video AS (
			UPDATE videos v SET status = 'FAILED', failure_code = $3, failure_message = $4, updated_at = $5
			FROM processing_jobs j WHERE j.id = $1 AND j.run_id = $2 AND v.id = j.video_id
			  AND v.processing_version = j.processing_version AND v.status IN ('QUEUED', 'PROCESSING')
			RETURNING v.id
		)
		UPDATE processing_jobs SET status = 'FAILED', lease_until = NULL, finished_at = $5,
			error_code = $3, error_message = $4, updated_at = $5
		WHERE id = $1 AND run_id = $2 AND status = 'RUNNING'
		  AND EXISTS (SELECT 1 FROM failed_video)
	`, job.ID, job.RunID, code, message, now)
	if err != nil {
		return err
	}
	if err := expectOneRow(result); err != nil {
		return err
	}
	if len(original) == 0 {
		original = []byte("{}")
	}
	if err := insertDeadLetter(ctx, tx, job.ID, job.ID, original, code, message, attempt, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *Repository) RecordOperation(ctx context.Context, job domain.Job, stage, variant, objectKey string, now time.Time) error {
	key := fmt.Sprintf("%s:%s:v%d:%s", job.VideoID, stage, job.ProcessingVersion, variant)
	result, err := repository.db.ExecContext(ctx, `
		INSERT INTO processing_operations (operation_key, job_id, stage, variant, status, run_id, object_key, updated_at)
		SELECT $1, j.id, $4, $5, 'SUCCEEDED', $3, $6, $7
		FROM processing_jobs j JOIN videos v ON v.id = j.video_id
		WHERE j.id = $2 AND j.run_id = $3 AND j.status = 'RUNNING' AND j.lease_until > $7
		  AND v.processing_version = j.processing_version
		ON CONFLICT (operation_key) DO UPDATE SET status = 'SUCCEEDED', run_id = EXCLUDED.run_id,
			object_key = EXCLUDED.object_key, updated_at = EXCLUDED.updated_at
	`, key, job.ID, job.RunID, stage, variant, objectKey, now)
	if err != nil {
		return err
	}
	return expectOneRow(result)
}

func (repository *Repository) Quarantine(ctx context.Context, body []byte, code, message string, now time.Time) error {
	hash := sha256.Sum256(body)
	dedupe := "invalid:" + hex.EncodeToString(hash[:])
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertDeadLetter(ctx, tx, "", dedupe, body, code, message, 0, now); err != nil {
		return err
	}
	return tx.Commit()
}

func insertDeadLetter(ctx context.Context, tx *sql.Tx, jobID, dedupe string, body []byte, code, message string, attempt int, now time.Time) error {
	id := uuid.NewString()
	var jobValue any
	if jobID != "" {
		jobValue = jobID
	}
	_, err := tx.ExecContext(ctx, `
		WITH inserted AS (
			INSERT INTO processing_dead_letters
				(id, dedupe_key, job_id, original_message, error_code, error_message, attempt, failed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (dedupe_key) DO NOTHING RETURNING id
		)
		INSERT INTO outbox_events (id, event_key, exchange_name, routing_key, payload)
		SELECT id, $2 || ':dead', 'processing.dead', 'processing.dead.v1',
			jsonb_build_object('message_id', id, 'message_type', 'processing.dead.v1',
				'original_message', encode($4::bytea, 'base64'), 'error_code', $5,
				'error_message', $6, 'attempt', $7, 'failed_at', $8)
		FROM inserted ON CONFLICT (event_key) DO NOTHING
	`, id, dedupe, jobValue, body, code, message, attempt, now)
	return err
}

func (repository *Repository) CleanupCandidates(ctx context.Context, before time.Time) ([]ports.RunCleanup, error) {
	rows, err := repository.db.QueryContext(ctx, `SELECT id::text, object_prefix FROM processing_runs r
		WHERE status = 'FAILED' AND finished_at < $1 AND cleaned_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM assets a WHERE a.object_key LIKE r.object_prefix || '%')
		ORDER BY finished_at LIMIT 20`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []ports.RunCleanup
	for rows.Next() {
		var run ports.RunCleanup
		if err := rows.Scan(&run.ID, &run.Prefix); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (repository *Repository) MarkRunClean(ctx context.Context, runID string, now time.Time) error {
	_, err := repository.db.ExecContext(ctx, `UPDATE processing_runs SET cleaned_at = $2
		WHERE id = $1 AND status = 'FAILED' AND cleaned_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM assets a WHERE a.object_key LIKE processing_runs.object_prefix || '%')`, runID, now)
	return err
}

var _ ports.ReliableRepository = (*Repository)(nil)
