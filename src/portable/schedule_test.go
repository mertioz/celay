package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestQueueMigratesExistingJobsAndPersistsTurnOptions(t *testing.T) {
	folder := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(folder, "schedule.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_, err = db.Exec(`
		CREATE TABLE jobs (
			id TEXT PRIMARY KEY, thread_id TEXT NOT NULL, title TEXT NOT NULL,
			prompt TEXT NOT NULL, due INTEGER NOT NULL, next_attempt INTEGER NOT NULL,
			created INTEGER NOT NULL, state TEXT NOT NULL DEFAULT 'queued',
			detail TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO jobs(id, thread_id, title, prompt, due, next_attempt, created)
		VALUES ('old', 'old-thread', 'Old title', 'Old prompt', ?, ?, ?);
	`, now.Add(-time.Minute).UnixMilli(), now.UnixMilli(), now.UnixMilli())
	db.Close()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	queue, err := openQueue(folder)
	if err != nil {
		t.Fatal(err)
	}
	selection := turnOptions{Model: "chosen", Effort: "high"}
	err = queue.add(ctx, discussion{ID: "new-thread", Title: "New title"}, "New prompt", now, selection)
	queue.db.Close()
	if err != nil {
		t.Fatal(err)
	}
	queue, err = openQueue(folder)
	if err != nil {
		t.Fatal(err)
	}
	defer queue.db.Close()
	jobs, err := queue.list(ctx)
	if err != nil || len(jobs) != 2 || jobs[0].ID != "old" || jobs[0].Prompt != "Old prompt" ||
		jobs[0].Options != (turnOptions{}) || jobs[1].Options != selection {
		t.Fatalf("unexpected jobs after migration and reopen: %+v, error %v", jobs, err)
	}
	if old, err := queue.claim(ctx, now); err != nil || old.ID != "old" || old.Options != (turnOptions{}) {
		t.Fatalf("unexpected migrated job: %+v, error %v", old, err)
	}
	if next, err := queue.claim(ctx, now); err != nil || next.ThreadID != "new-thread" || next.Options != selection {
		t.Fatalf("worker did not receive the stored options: %+v, error %v", next, err)
	}
}
