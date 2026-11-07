package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
)

func TestScheduledDeletion(t *testing.T) {
	ctx := context.Background()
	queue, err := openQueue(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer queue.db.Close()
	thread := discussion{ID: "test-thread", Title: "Discussion"}
	now := time.Now()
	if err := queue.add(ctx, thread, "remove this message", now); err != nil {
		t.Fatal(err)
	}
	jobs, err := queue.list(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("expected one scheduled message: %v, %v", jobs, err)
	}
	if removed, err := queue.remove(ctx, jobs[0].ID); err != nil || !removed {
		t.Fatalf("scheduled message was not removed: %v", err)
	}
	if _, err := queue.claim(ctx, now.Add(time.Minute)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted message was available for dispatch: %v", err)
	}

	if err := queue.add(ctx, thread, "keep the active send", now); err != nil {
		t.Fatal(err)
	}
	sending, err := queue.claim(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if removed, err := queue.remove(ctx, sending.ID); err != nil || removed {
		t.Fatalf("in-flight message was removable: %v", err)
	}
	queue.write("UPDATE jobs SET state='sent' WHERE id=?", sending.ID)
	if removed, err := queue.remove(ctx, sending.ID); err != nil || !removed {
		t.Fatalf("completed entry was not removable: %v", err)
	}
}

func TestDesktopSelectionShowsActions(t *testing.T) {
	application := test.NewApp()
	ui := newDesktop(context.Background(), application, nil, "")
	defer ui.window.Close()
	ui.discussions = []discussion{{ID: "one", Title: "Échange de test"}}
	ui.filter("echange")
	if ui.threadActions.Visible() || len(ui.filtered) != 1 {
		t.Fatal("actions must start hidden and the filter must ignore accents")
	}
	ui.threadList.Select(0)
	if !ui.threadActions.Visible() || ui.thread == nil || ui.thread.ID != "one" {
		t.Fatal("selecting a discussion must show its actions")
	}
	ui.jobs = []job{{ID: "queued", State: "queued"}, {ID: "sending", State: "sending"}}
	ui.jobList.Refresh()
	ui.jobList.Select(0)
	if !ui.jobActions.Visible() || ui.deleteButton.Disabled() {
		t.Fatal("a scheduled entry must expose deletion")
	}
	ui.jobList.Select(1)
	if !ui.deleteButton.Disabled() {
		t.Fatal("deletion must be disabled once a send is in progress")
	}
}

func TestMarkdownDownloadPreservesAnExistingFile(t *testing.T) {
	folder := t.TempDir()
	first, err := writeMarkdown(folder, "CON.md", []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := writeMarkdown(folder, "CON.md", []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(first)
	if err != nil || string(original) != "original" || filepath.Base(second) != "Celay - CON (2).md" {
		t.Fatalf("unexpected output names or content: %s, %s, %v", first, second, err)
	}
}
