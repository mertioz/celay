package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/text/unicode/norm"
)

type desktopUI struct {
	ctx         context.Context
	application fyne.App
	window      fyne.Window
	queue       *queue
	root        string

	discussions []discussion
	filtered    []discussion
	jobs        []job
	thread      *discussion
	selectedJob *job

	search                       *widget.Entry
	threadList, jobList          *widget.List
	count, jobCount, status      *widget.Label
	threadTitle                  *widget.Label
	threadActions, jobActions    *fyne.Container
	downloadButton, deleteButton *widget.Button
}

func newDesktop(ctx context.Context, application fyne.App, queue *queue, root string) *desktopUI {
	ui := &desktopUI{
		ctx: ctx, application: application, queue: queue, root: root,
		window: application.NewWindow("Celay"),
	}
	ui.status = shortLabel("Les envois programmés restent actifs quand la fenêtre est masquée.")
	heading := widget.NewLabelWithStyle("Celay", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	quit := widget.NewButton("Quitter", ui.quit)
	top := container.NewBorder(nil, nil, nil, quit, heading)
	split := container.NewHSplit(ui.discussionPane(), ui.jobPane())
	split.Offset = .56
	ui.window.SetContent(container.NewPadded(container.NewBorder(
		top, container.NewVBox(widget.NewSeparator(), ui.status), nil, nil, split)))
	ui.window.Resize(fyne.NewSize(940, 620))
	ui.window.CenterOnScreen()
	ui.window.SetCloseIntercept(ui.closeWindow)
	ui.window.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("Celay",
		fyne.NewMenuItem("Actualiser les discussions", ui.loadDiscussions),
		fyne.NewMenuItem("Quitter", ui.quit))))
	if systemTray, ok := application.(desktop.App); ok {
		systemTray.SetSystemTrayIcon(application.Icon())
		systemTray.SetSystemTrayMenu(fyne.NewMenu("Celay",
			fyne.NewMenuItem("Afficher Celay", ui.show),
			fyne.NewMenuItem("Quitter Celay", ui.quit)))
	}
	return ui
}

func shortLabel(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Truncation = fyne.TextTruncateEllipsis
	return label
}

func panelHeading(title string) *widget.Label {
	return widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

func (ui *desktopUI) discussionPane() fyne.CanvasObject {
	ui.search = widget.NewEntry()
	ui.search.SetPlaceHolder("Filtrer par titre…")
	ui.count = shortLabel("Chargement des discussions…")
	ui.threadList = widget.NewList(
		func() int { return len(ui.filtered) },
		func() fyne.CanvasObject { return shortLabel("") },
		func(id widget.ListItemID, item fyne.CanvasObject) {
			thread := ui.filtered[id]
			title := thread.Title
			if thread.Archived {
				title += "   ·   Archivée"
			}
			item.(*widget.Label).SetText(title)
		},
	)
	ui.threadTitle = shortLabel("")
	ui.downloadButton = widget.NewButtonWithIcon("Télécharger", theme.DownloadIcon(), ui.download)
	compose := widget.NewButtonWithIcon("Envoyer un message", theme.MailSendIcon(), ui.compose)
	compose.Importance = widget.HighImportance
	ui.threadActions = container.NewVBox(widget.NewSeparator(), ui.threadTitle,
		container.NewHBox(ui.downloadButton, compose))
	ui.threadActions.Hide()
	ui.threadList.OnSelected = func(id widget.ListItemID) {
		thread := ui.filtered[id]
		ui.thread = &thread
		ui.threadTitle.SetText(thread.Title)
		ui.threadActions.Show()
	}
	ui.threadList.OnUnselected = func(widget.ListItemID) {
		ui.thread = nil
		ui.threadActions.Hide()
	}
	ui.search.OnChanged = ui.filter
	refresh := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), ui.loadDiscussions)
	header := container.NewVBox(
		container.NewBorder(nil, nil, nil, refresh, panelHeading("Discussions Codex")),
		ui.search, ui.count)
	return container.NewPadded(container.NewBorder(header, ui.threadActions, nil, nil, ui.threadList))
}

func (ui *desktopUI) jobPane() fyne.CanvasObject {
	ui.jobCount = shortLabel("Aucun envoi programmé.")
	ui.jobList = widget.NewList(
		func() int { return len(ui.jobs) },
		func() fyne.CanvasObject { return container.NewVBox(shortLabel(""), shortLabel("")) },
		func(id widget.ListItemID, item fyne.CanvasObject) {
			job := ui.jobs[id]
			row := item.(*fyne.Container)
			row.Objects[0].(*widget.Label).SetText(job.Title)
			row.Objects[1].(*widget.Label).SetText(job.Due.Local().Format("02/01/2006 à 15:04") + "   ·   " + jobStatus(job))
		},
	)
	preview := widget.NewButton("Voir le message", ui.previewJob)
	ui.deleteButton = widget.NewButtonWithIcon("Supprimer", theme.DeleteIcon(), ui.deleteJob)
	ui.deleteButton.Importance = widget.DangerImportance
	ui.jobActions = container.NewVBox(widget.NewSeparator(), container.NewHBox(preview, ui.deleteButton))
	ui.jobActions.Hide()
	ui.jobList.OnSelected = func(id widget.ListItemID) {
		job := ui.jobs[id]
		ui.selectedJob = &job
		ui.deleteButton.Enable()
		if job.State == "sending" {
			ui.deleteButton.Disable()
		}
		ui.jobActions.Show()
	}
	ui.jobList.OnUnselected = func(widget.ListItemID) {
		ui.selectedJob = nil
		ui.jobActions.Hide()
	}
	header := container.NewVBox(panelHeading("Envois programmés"), ui.jobCount)
	return container.NewPadded(container.NewBorder(header, ui.jobActions, nil, nil, ui.jobList))
}

func (ui *desktopUI) loadDiscussions() {
	go func() {
		items, err := listDiscussions(ui.ctx, ui.root)
		if ui.ctx.Err() != nil {
			return
		}
		fyne.Do(func() {
			if err != nil {
				ui.count.SetText("Impossible de lire l’historique Codex.")
				if errors.Is(err, os.ErrNotExist) {
					ui.count.SetText("Aucun historique Codex trouvé sur cet ordinateur.")
				}
				return
			}
			ui.discussions = items
			ui.filter(ui.search.Text)
		})
	}()
}

func searchText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return unicode.ToLower(r)
	}, norm.NFD.String(text))
}

func (ui *desktopUI) filter(text string) {
	var selected string
	if ui.thread != nil {
		selected = ui.thread.ID
	}
	ui.threadList.UnselectAll()
	ui.filtered = nil
	query := searchText(strings.TrimSpace(text))
	for _, item := range ui.discussions {
		if strings.Contains(searchText(item.Title), query) {
			ui.filtered = append(ui.filtered, item)
		}
	}
	count := len(ui.filtered)
	label := "discussions"
	if count == 1 {
		label = "discussion"
	}
	ui.count.SetText(fmt.Sprintf("%d %s", count, label))
	ui.threadList.Refresh()
	for index, thread := range ui.filtered {
		if thread.ID == selected {
			ui.threadList.Select(index)
			break
		}
	}
}

func (ui *desktopUI) download() {
	if ui.thread == nil {
		return
	}
	thread := *ui.thread
	ui.downloadButton.Disable()
	ui.status.SetText("Export en cours…")
	go func() {
		path, err := saveDiscussion(ui.ctx, ui.root, thread)
		if ui.ctx.Err() != nil {
			return
		}
		fyne.Do(func() {
			ui.downloadButton.Enable()
			if err != nil {
				ui.status.SetText("Export impossible.")
				dialog.ShowError(err, ui.window)
				return
			}
			ui.status.SetText("Enregistré dans Téléchargements · " + filepath.Base(path))
		})
	}()
}

func (ui *desktopUI) compose() {
	if ui.thread == nil {
		return
	}
	thread := *ui.thread
	if thread.Archived {
		dialog.ShowInformation("Discussion archivée",
			"Désarchive cette discussion dans Codex avant de programmer un envoi.", ui.window)
		return
	}
	prompt := widget.NewMultiLineEntry()
	prompt.SetPlaceHolder("Écris ton prompt…")
	prompt.SetMinRowsVisible(7)
	date := widget.NewDateEntry()
	hour := widget.NewEntry()
	hour.SetPlaceHolder("HH:MM")
	next := time.Now().Add(time.Minute).Truncate(time.Minute)
	date.SetDate(&next)
	hour.SetText(next.Format("15:04"))
	picker := newModelPicker()
	catalogStatus := widget.NewLabel("Chargement du catalogue Codex…")
	catalogStatus.Wrapping = fyne.TextWrapWord
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	submit := widget.NewButton("Programmer l’envoi", nil)
	submit.Importance = widget.HighImportance
	submit.OnTapped = func() {
		if strings.TrimSpace(prompt.Text) == "" || len(prompt.Text) > 240<<10 {
			status.SetText("Écris un message de 240 Kio maximum.")
			return
		}
		due, err := scheduleTime(date.Date, hour.Text)
		if err != nil {
			status.SetText(err.Error())
			return
		}
		message := prompt.Text
		options := picker.selection()
		submit.Disable()
		status.SetText("Enregistrement…")
		go func() {
			ctx, cancel := context.WithTimeout(ui.ctx, 12*time.Second)
			defer cancel()
			_, err := locateCodex(ctx)
			if err == nil {
				if due.After(time.Now()) {
					err = ui.queue.add(ctx, thread, message, due, options)
				} else {
					err = errors.New("Choisis une heure à venir.")
				}
			}
			if ui.ctx.Err() != nil {
				return
			}
			fyne.Do(func() {
				submit.Enable()
				if err != nil {
					status.SetText(err.Error())
					return
				}
				if prompt.Text == message {
					prompt.SetText("")
				}
				status.SetText("Message programmé. Tu peux en ajouter un autre.")
				ui.refreshJobs()
			})
		}()
	}
	inputs := container.NewGridWithColumns(2,
		container.NewVBox(shortLabel("Date"), date),
		container.NewVBox(shortLabel("Heure locale · HH:MM"), hour))
	selectors := container.NewGridWithColumns(2,
		container.NewVBox(shortLabel("Modèle"), picker.model),
		container.NewVBox(shortLabel("Raisonnement"), picker.effort))
	title := shortLabel(thread.Title)
	title.TextStyle.Bold = true
	content := container.NewVBox(title, widget.NewLabel("Message"), prompt, inputs, selectors, catalogStatus, status, submit)
	form := dialog.NewCustom("Envoyer un message", "Fermer", content, ui.window)
	catalogCtx, closeCatalog := context.WithCancel(ui.ctx)
	form.SetOnClosed(closeCatalog)
	form.Resize(fyne.NewSize(540, 475))
	form.Show()
	ui.window.Canvas().Focus(prompt)
	go func() {
		ctx, cancel := context.WithTimeout(catalogCtx, 15*time.Second)
		defer cancel()
		models, err := loadModelCatalog(ctx)
		if catalogCtx.Err() != nil {
			return
		}
		fyne.Do(func() {
			if catalogCtx.Err() != nil {
				return
			}
			if err != nil {
				catalogStatus.SetText("Choix du modèle indisponible. " + err.Error())
			} else if len(models) == 0 {
				catalogStatus.SetText("Codex ne propose aucun modèle de texte dans son catalogue.")
			} else {
				picker.setCatalog(models)
				catalogStatus.Hide()
			}
		})
	}()
}

func scheduleTime(day *time.Time, hour string) (time.Time, error) {
	clock, err := time.Parse("15:04", strings.TrimSpace(hour))
	if err != nil || day == nil {
		return time.Time{}, errors.New("Choisis une date et saisis l’heure au format HH:MM.")
	}
	due := time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, time.Local)
	if due.Hour() != clock.Hour() || due.Minute() != clock.Minute() || !due.After(time.Now()) {
		return time.Time{}, errors.New("Choisis une date et une heure locales à venir.")
	}
	return due, nil
}

func jobStatus(job job) string {
	switch job.State {
	case "queued":
		if job.Detail != "" {
			return "Discussion occupée"
		}
		return "Programmé"
	case "sending":
		return "En cours"
	case "sent":
		return "Envoyé"
	case "failed":
		return "Échec"
	case "uncertain":
		return "À vérifier"
	default:
		return job.State
	}
}

func (ui *desktopUI) refreshJobs() {
	go func() {
		items, err := ui.queue.list(ui.ctx)
		if ui.ctx.Err() != nil {
			return
		}
		fyne.Do(func() {
			if err != nil {
				ui.jobCount.SetText("Impossible de lire les envois programmés.")
				return
			}
			if len(items) == 0 {
				ui.jobCount.SetText("Aucun envoi programmé.")
			} else {
				ui.jobCount.SetText(fmt.Sprintf("%d envoi(s)", len(items)))
			}
			if slices.Equal(ui.jobs, items) {
				return
			}
			var selected string
			if ui.selectedJob != nil {
				selected = ui.selectedJob.ID
			}
			ui.jobList.UnselectAll()
			ui.jobs = items
			ui.jobList.Refresh()
			for index, item := range items {
				if item.ID == selected {
					ui.jobList.Select(index)
					break
				}
			}
		})
	}()
}

func (ui *desktopUI) previewJob() {
	if ui.selectedJob == nil {
		return
	}
	job := *ui.selectedJob
	prompt := widget.NewLabel(job.Prompt)
	prompt.Wrapping = fyne.TextWrapWord
	preview := container.NewVScroll(prompt)
	preview.SetMinSize(fyne.NewSize(400, 120))
	details := widget.NewLabel(turnDetail(job.TurnID, job.Detail))
	details.Wrapping = fyne.TextWrapWord
	header := container.NewVBox(shortLabel(job.Title),
		shortLabel(job.Due.Local().Format("02/01/2006 à 15:04")+" · "+jobStatus(job)))
	if job.Options.Model != "" {
		effort := defaultEffortLabel
		if job.Options.Effort != "" {
			effort = effortLabel(job.Options.Effort)
		}
		header.Add(shortLabel(job.Options.Model + " · Raisonnement " + effort))
	}
	if job.Detail != "" {
		header.Add(details)
	}
	info := dialog.NewCustom("Message programmé", "Fermer",
		container.NewBorder(header, nil, nil, nil, preview), ui.window)
	info.Resize(fyne.NewSize(500, 330))
	info.Show()
}

func (ui *desktopUI) deleteJob() {
	if ui.selectedJob == nil || ui.selectedJob.State == "sending" {
		return
	}
	job := *ui.selectedJob
	message := "Cette entrée sera effacée de Celay."
	if job.State == "queued" {
		message = "Cet envoi sera retiré de la file. Le message ne partira pas."
	}
	confirm := dialog.NewConfirm("Supprimer ce message ?", message, func(yes bool) {
		if !yes {
			return
		}
		removed, err := ui.queue.remove(ui.ctx, job.ID)
		if err != nil {
			dialog.ShowError(err, ui.window)
		} else if !removed {
			dialog.ShowInformation("Envoi en cours", "Ce message est déjà en cours d’envoi.", ui.window)
		} else {
			ui.status.SetText("Message supprimé de la file Celay.")
		}
		ui.refreshJobs()
	}, ui.window)
	confirm.SetConfirmText("Supprimer")
	confirm.SetDismissText("Garder")
	confirm.Show()
}

func (ui *desktopUI) show() {
	ui.window.Show()
	ui.window.RequestFocus()
}

func (ui *desktopUI) closeWindow() {
	active, err := ui.queue.active(ui.ctx)
	if err != nil || active {
		ui.window.Hide()
		return
	}
	ui.application.Quit()
}

func (ui *desktopUI) quit() {
	active, err := ui.queue.active(ui.ctx)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	if !active {
		ui.application.Quit()
		return
	}
	ui.show()
	confirm := dialog.NewConfirm("Des envois sont en attente",
		"Les envois en attente reprendront à la prochaine ouverture de Celay. Un envoi en cours peut demander une vérification.",
		func(yes bool) {
			if yes {
				ui.application.Quit()
			}
		}, ui.window)
	confirm.SetConfirmText("Quitter Celay")
	confirm.SetDismissText("Laisser tourner")
	confirm.Show()
}

func (ui *desktopUI) watch(signal string) {
	ui.loadDiscussions()
	ui.refreshJobs()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ui.ctx.Done():
				return
			case <-tick.C:
				if os.Remove(signal) == nil {
					fyne.Do(ui.show)
				}
				ui.refreshJobs()
			}
		}
	}()
}
