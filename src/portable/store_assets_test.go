//go:build storeassets

package main

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

// TestExportStoreAssets captures the actual Fyne widgets with example data.
// Run explicitly with -tags "ci storeassets" and CELAY_STORE_ASSETS_DIR.
func TestExportStoreAssets(t *testing.T) {
	root := os.Getenv("CELAY_STORE_ASSETS_DIR")
	if root == "" {
		t.Skip("set CELAY_STORE_ASSETS_DIR to export Store images")
	}
	application := test.NewApp()
	defer application.Quit()
	application.Settings().SetTheme(theme.LightTheme())
	icon := fyne.NewStaticResource("celay.svg", iconData)
	for _, asset := range []struct {
		name string
		size float32
	}{{"StoreLogo.png", 50}, {"Square44x44Logo.png", 44}, {"Square150x150Logo.png", 150}, {"ListingLogo.png", 300}} {
		window := application.NewWindow("Celay")
		image := canvas.NewImageFromResource(icon)
		image.FillMode = canvas.ImageFillContain
		window.SetContent(image)
		window.Resize(fyne.NewSize(asset.size, asset.size))
		window.Show()
		writeScreenshot(t, window, filepath.Join(root, "icons", asset.name))
		window.Close()
	}

	queue, err := openQueue(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer queue.db.Close()
	ui := newDesktop(context.Background(), application, queue, "")
	ui.discussions = []discussion{
		{ID: "docs", Title: "Préparer la documentation du projet"},
		{ID: "release", Title: "Organiser la prochaine version"},
		{ID: "compare", Title: "Comparer deux approches"},
		{ID: "ideas", Title: "Idées pour une application"},
		{ID: "archive", Title: "Notes de recherche", Archived: true},
	}
	ui.filter("")
	ui.threadList.Select(0)
	due := time.Date(2026, 11, 20, 15, 30, 0, 0, time.Local)
	ui.jobs = []job{
		{ID: "one", ThreadID: "docs", Title: ui.discussions[0].Title,
			Prompt: "Prépare une synthèse des changements et propose les sections à ajouter à la documentation.",
			Due:    due, State: "queued", Options: turnOptions{Model: "gpt-6.1-sol", Effort: "high"}},
		{ID: "two", ThreadID: "release", Title: ui.discussions[1].Title,
			Prompt: "Prépare la liste des points à vérifier avant la prochaine version.",
			Due:    due.Add(3 * time.Hour), State: "queued"},
	}
	ui.jobCount.SetText("2 envois")
	ui.jobList.Refresh()
	ui.jobList.Select(0)
	ui.window.Resize(fyne.NewSize(1440, 900))
	ui.window.Show()
	writeScreenshot(t, ui.window, filepath.Join(root, "screenshots", "01-discussions-et-envois.png"))
	ui.previewJob()
	writeScreenshot(t, ui.window, filepath.Join(root, "screenshots", "02-detail-envoi.png"))
	ui.window.SetCloseIntercept(nil)
	ui.window.Close()
}

func writeScreenshot(t *testing.T, window fyne.Window, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, window.Canvas().Capture())
	closeErr := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}
