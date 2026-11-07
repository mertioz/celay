package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const version = "2.1.0"

//go:embed celay.svg
var iconData []byte

func dataFolder() (string, error) {
	if folder := os.Getenv("CELAY_DATA_DIR"); folder != "" {
		return filepath.Abs(folder)
	}
	parent, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, "Celay"), nil
}

func instance(folder string) (*os.File, error) {
	if err := os.MkdirAll(folder, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(folder, "instance.lock"), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	locked, err := lockFile(file)
	if err != nil {
		file.Close()
		return nil, err
	}
	legacy := filepath.Join(folder, "instance.url")
	if !locked {
		file.Close()
		if _, err := os.Stat(legacy); err == nil {
			return nil, errors.New("Quitte l’ancienne version de Celay avant d’ouvrir cette version.")
		}
		return nil, os.WriteFile(filepath.Join(folder, "show-window"), nil, 0600)
	}
	// A crash of Celay 2.0 may have left its browser address behind.
	os.Remove(legacy)
	return file, nil
}

func run() error {
	root, err := codexHome()
	if err != nil {
		return err
	}
	folder, err := dataFolder()
	if err != nil {
		return err
	}
	lock, err := instance(folder)
	if err != nil || lock == nil {
		return err
	}
	defer lock.Close()
	queue, err := openQueue(folder)
	if err != nil {
		return err
	}
	defer queue.db.Close()

	application := app.NewWithID("local.celay.app")
	application.SetIcon(fyne.NewStaticResource("celay.svg", iconData))
	ctx, cancel := context.WithCancel(context.Background())
	workersDone := make(chan struct{})
	go func() {
		defer close(workersDone)
		queue.run(ctx, func(ctx context.Context, item job, accepted func(string)) delivery {
			return sendToCodex(ctx, root, item, accepted)
		})
	}()

	ui := newDesktop(ctx, application, queue, root)
	ui.watch(filepath.Join(folder, "show-window"))
	ui.window.ShowAndRun()
	cancel()
	<-workersDone
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Celay :", err)
		application := app.NewWithID("local.celay.app")
		window := application.NewWindow("Celay")
		message := widget.NewLabel(err.Error())
		message.Wrapping = fyne.TextWrapWord
		window.SetContent(container.NewPadded(container.NewVBox(
			widget.NewLabelWithStyle("Impossible d’ouvrir Celay", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			message, widget.NewButton("Fermer", window.Close))))
		window.Resize(fyne.NewSize(430, 140))
		window.CenterOnScreen()
		window.ShowAndRun()
	}
}
