package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

type job struct {
	ID, ThreadID, Title, Prompt, State, Detail, TurnID string
	Due                                                time.Time
}

type queue struct {
	db   *sql.DB
	wake chan struct{}
}

func openQueue(folder string) (*queue, error) {
	db, err := sql.Open("sqlite", filepath.Join(folder, "schedule.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY, thread_id TEXT NOT NULL, title TEXT NOT NULL,
			prompt TEXT NOT NULL, due INTEGER NOT NULL, next_attempt INTEGER NOT NULL,
			created INTEGER NOT NULL, state TEXT NOT NULL DEFAULT 'queued',
			detail TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS jobs_due ON jobs(state, next_attempt);
		CREATE INDEX IF NOT EXISTS jobs_thread ON jobs(thread_id, state);
		UPDATE jobs SET state = 'uncertain', detail = CASE WHEN turn_id = ''
			THEN 'Celay a été interrompu pendant l’envoi. Vérifie la discussion avant de reprogrammer.'
			ELSE 'Codex a accepté le message. Vérifie sa réponse dans la discussion.' END
		WHERE state = 'sending';`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &queue{db: db, wake: make(chan struct{}, 1)}, nil
}

func (q *queue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *queue) add(ctx context.Context, thread discussion, prompt string, due time.Time) error {
	at := due.UnixMilli()
	_, err := q.db.ExecContext(ctx, `INSERT INTO jobs(id, thread_id, title, prompt, due, next_attempt, created)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, rand.Text(), thread.ID, thread.Title, prompt, at, at, time.Now().UnixMilli())
	q.signal()
	return err
}

func (q *queue) cancel(ctx context.Context, id string) (bool, error) {
	result, err := q.db.ExecContext(ctx, "DELETE FROM jobs WHERE id = ? AND state = 'queued'", id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows != 0, err
}

const jobColumns = "id, thread_id, title, prompt, state, detail, turn_id, due"

func scanJob(row interface{ Scan(...any) error }) (job, error) {
	var item job
	var due int64
	err := row.Scan(&item.ID, &item.ThreadID, &item.Title, &item.Prompt,
		&item.State, &item.Detail, &item.TurnID, &due)
	item.Due = time.UnixMilli(due)
	return item, err
}

func (q *queue) list(ctx context.Context) ([]job, error) {
	rows, err := q.db.QueryContext(ctx, "SELECT "+jobColumns+` FROM jobs ORDER BY
		CASE WHEN state IN ('queued', 'sending') THEN 0 ELSE 1 END,
		CASE WHEN state IN ('queued', 'sending') THEN due ELSE -due END, created, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []job
	for rows.Next() {
		item, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *queue) active(ctx context.Context) (bool, error) {
	var active bool
	err := q.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE state IN ('queued', 'sending'))").Scan(&active)
	return active, err
}

func (q *queue) claim(ctx context.Context, now time.Time) (job, error) {
	// Once the oldest job for a thread has been claimed or delayed, later jobs
	// for that thread wait behind it. Other threads can still run independently.
	return scanJob(q.db.QueryRowContext(ctx, `UPDATE jobs SET state='sending', detail=''
		WHERE id = (
			SELECT current.id FROM jobs current
			WHERE current.state='queued' AND current.next_attempt <= ?
				AND NOT EXISTS (SELECT 1 FROM jobs earlier
					WHERE earlier.thread_id=current.thread_id AND (
						earlier.state='sending' OR
						(earlier.state='queued' AND (earlier.due, earlier.created, earlier.id) < (current.due, current.created, current.id))
					))
			ORDER BY current.due, current.created, current.id LIMIT 1
		) RETURNING `+jobColumns, now.UnixMilli()))
}

type delivery struct {
	state, detail, turnID string
}

type deliverFunc func(context.Context, job, func(string)) delivery

func (q *queue) run(ctx context.Context, send deliverFunc) {
	const parallelThreads = 4
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	finished := make(chan struct{}, parallelThreads)
	running := 0
	for {
		for ctx.Err() == nil && running < parallelThreads {
			item, err := q.claim(ctx, time.Now())
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
					fmt.Println("Celay : impossible de lire la file d’envoi.", err)
				}
				break
			}
			running++
			go func() {
				defer func() { finished <- struct{}{} }()
				result := send(ctx, item, func(turnID string) {
					q.write("UPDATE jobs SET turn_id=? WHERE id=?", turnID, item.ID)
				})
				if result.state == "retry" {
					q.write("UPDATE jobs SET state='queued', next_attempt=?, detail=? WHERE id=?",
						time.Now().Add(15*time.Second).UnixMilli(), result.detail, item.ID)
				} else {
					q.write("UPDATE jobs SET state=?, detail=?, turn_id=? WHERE id=?",
						result.state, result.detail, result.turnID, item.ID)
				}
			}()
		}
		select {
		case <-finished:
			running--
		case <-ticker.C:
		case <-q.wake:
		case <-ctx.Done():
			for range running {
				<-finished
			}
			return
		}
	}
}

func (q *queue) write(statement string, args ...any) {
	if _, err := q.db.Exec(statement, args...); err != nil {
		fmt.Println("Celay : impossible d’enregistrer l’état de l’envoi.", err)
	}
}
