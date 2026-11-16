// Package jobs is a small durable job queue on Postgres.
//
// Workers claim jobs with SELECT ... FOR UPDATE SKIP LOCKED, so any number
// of workers can share the table. A job that stays "running" past the lock
// timeout is assumed orphaned by a crashed worker and is claimed again.
// The interface is deliberately close to River's, so it can be swapped for
// River later without touching handlers.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Job is a claimed job as a handler sees it.
type Job struct {
	ID          int64
	Kind        string
	Payload     json.RawMessage
	Attempt     int // 1 on the first run
	MaxAttempts int
}

// Final reports whether this is the last attempt.
func (j Job) Final() bool { return j.Attempt >= j.MaxAttempts }

// Handler runs one job. Its result is stored as JSON on success.
type Handler func(ctx context.Context, j Job) (any, error)

// Execer is satisfied by *sql.DB and *sql.Tx, so jobs can be enqueued in the
// same transaction as the change that needs them.
type Execer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Options for Enqueue.
type Options struct {
	MaxAttempts int           // default 2
	Delay       time.Duration // run no earlier than now + Delay
}

// Enqueue inserts a job and returns its id.
func Enqueue(ctx context.Context, q Execer, kind string, payload any, opt Options) (int64, error) {
	if opt.MaxAttempts == 0 {
		opt.MaxAttempts = 2
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var id int64
	err = q.QueryRowContext(ctx, `INSERT INTO jobs (kind, payload, max_attempts, run_after)
		VALUES ($1, $2, $3, now() + $4 * interval '1 millisecond') RETURNING id`,
		kind, b, opt.MaxAttempts, opt.Delay.Milliseconds()).Scan(&id)
	return id, err
}

// Worker claims and runs jobs.
type Worker struct {
	DB          *sql.DB
	ID          string
	Handlers    map[string]Handler
	Poll        time.Duration // default 1s
	LockTimeout time.Duration // default 30m; longer than any job should run
	// OnFailed runs once when a job fails for the last time, for example to
	// refund credits. Its error is logged, not retried.
	OnFailed func(ctx context.Context, j Job, err error) error
	Log      *slog.Logger
}

// ErrNoJob means there was nothing to claim.
var ErrNoJob = errors.New("no job ready")

// Run claims jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	poll := w.Poll
	if poll == 0 {
		poll = time.Second
	}
	for {
		err := w.RunOne(ctx)
		if errors.Is(err, ErrNoJob) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(poll):
			}
			continue
		}
		if err != nil {
			w.log().Error("job loop", "err", err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// RunOne claims and runs a single job. It returns ErrNoJob when the queue
// has nothing ready.
func (w *Worker) RunOne(ctx context.Context) error {
	j, err := w.claim(ctx)
	if err != nil {
		return err
	}
	h, ok := w.Handlers[j.Kind]
	var result any
	if !ok {
		err = fmt.Errorf("no handler for job kind %q", j.Kind)
	} else {
		result, err = safeRun(ctx, h, j)
	}
	if err == nil {
		return w.complete(ctx, j, result)
	}
	w.log().Warn("job failed", "id", j.ID, "kind", j.Kind, "attempt", j.Attempt, "err", err)
	return w.fail(ctx, j, err)
}

func safeRun(ctx context.Context, h Handler, j Job) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return h(ctx, j)
}

func (w *Worker) claim(ctx context.Context) (Job, error) {
	timeout := w.LockTimeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	var j Job
	err := w.DB.QueryRowContext(ctx, `UPDATE jobs SET status = 'running', progress = NULL, attempts = attempts + 1, locked_at = now(), locked_by = $1, updated_at = now()
		WHERE id = (
			SELECT id FROM jobs
			WHERE (status = 'queued' AND run_after <= now())
			   OR (status = 'running' AND locked_at < now() - $2 * interval '1 second')
			ORDER BY run_after, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1)
		RETURNING id, kind, payload, attempts, max_attempts`, w.id(), int(timeout.Seconds())).
		Scan(&j.ID, &j.Kind, &j.Payload, &j.Attempt, &j.MaxAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	return j, err
}

func (w *Worker) complete(ctx context.Context, j Job, result any) error {
	var b []byte
	if result != nil {
		var err error
		if b, err = json.Marshal(result); err != nil {
			return err
		}
	}
	_, err := w.DB.ExecContext(ctx, `UPDATE jobs SET status = 'succeeded', result = $2, last_error = NULL, locked_at = NULL, updated_at = now() WHERE id = $1`, j.ID, b)
	return err
}

func (w *Worker) fail(ctx context.Context, j Job, cause error) error {
	if !j.Final() {
		backoff := time.Duration(j.Attempt*j.Attempt) * 10 * time.Second
		_, err := w.DB.ExecContext(ctx, `UPDATE jobs SET status = 'queued', last_error = $2, locked_at = NULL,
			run_after = now() + $3 * interval '1 second', updated_at = now() WHERE id = $1`, j.ID, cause.Error(), int(backoff.Seconds()))
		return err
	}
	if _, err := w.DB.ExecContext(ctx, `UPDATE jobs SET status = 'failed', last_error = $2, locked_at = NULL, updated_at = now() WHERE id = $1`, j.ID, cause.Error()); err != nil {
		return err
	}
	if w.OnFailed != nil {
		if err := w.OnFailed(ctx, j, cause); err != nil {
			w.log().Error("job failure hook", "id", j.ID, "err", err)
		}
	}
	return nil
}

func (w *Worker) id() string {
	if w.ID != "" {
		return w.ID
	}
	host, _ := os.Hostname()
	return host + ":" + strconv.Itoa(os.Getpid())
}

func (w *Worker) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
