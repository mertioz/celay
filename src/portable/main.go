package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

//go:embed page.html
var pageSource string

var page = template.Must(template.New("index").Parse(pageSource))

const version = "2.0.0"

type app struct {
	root, host, prefix string
	queue              *queue
	lastSeen           atomic.Int64
	stop               chan struct{}
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	if r.Host != a.host || !strings.HasPrefix(r.URL.Path, a.prefix) {
		http.NotFound(w, r)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+a.host {
		http.Error(w, "Accès refusé.", http.StatusForbidden)
		return
	}
	a.lastSeen.Store(time.Now().Unix())
	path := strings.TrimPrefix(r.URL.Path, a.prefix)
	switch {
	case r.Method == http.MethodGet && path == "":
		a.showPage(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "export/"):
		a.download(w, r, strings.TrimPrefix(path, "export/"))
	case r.Method == http.MethodGet && path == "jobs":
		a.listJobs(w, r)
	case r.Method == http.MethodPost && path == "jobs":
		a.addJob(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "jobs/"):
		a.cancelJob(w, r, strings.TrimPrefix(path, "jobs/"))
	case r.Method == http.MethodGet && path == "health":
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && path == "alive":
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && path == "close":
		active, err := a.queue.active(r.Context())
		if err != nil || active {
			http.Error(w, "Celay reste actif pour les messages programmés. Tu peux fermer cet onglet.", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html lang="fr"><meta charset="utf-8"><title>Celay</title><p>Celay est fermé. Tu peux fermer cet onglet.</p></html>`)
		select {
		case a.stop <- struct{}{}:
		default:
		}
	default:
		http.NotFound(w, r)
	}
}

func (a *app) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := a.queue.list(r.Context())
	if err != nil {
		http.Error(w, "Impossible de lire les envois programmés.", http.StatusInternalServerError)
		return
	}
	if jobs == nil {
		jobs = []job{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(jobs)
}

func (a *app) addJob(w http.ResponseWriter, r *http.Request) {
	var data struct{ ThreadID, Prompt, Due string }
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Le message est invalide ou trop long.", http.StatusBadRequest)
		return
	}
	due, err := time.Parse(time.RFC3339Nano, data.Due)
	if err != nil || !due.After(time.Now()) {
		http.Error(w, "Choisis une date et une heure à venir.", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(data.Prompt) == "" {
		http.Error(w, "Écris le message à envoyer.", http.StatusBadRequest)
		return
	}
	thread, err := findDiscussion(r.Context(), a.root, data.ThreadID)
	if err != nil {
		http.Error(w, "Discussion introuvable.", http.StatusNotFound)
		return
	}
	if thread.Archived {
		http.Error(w, "Désarchive la discussion dans Codex avant de programmer un message.", http.StatusConflict)
		return
	}
	if _, err := locateCodex(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err := a.queue.add(r.Context(), thread, data.Prompt, due); err != nil {
		http.Error(w, "Impossible d’enregistrer l’envoi.", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *app) cancelJob(w http.ResponseWriter, r *http.Request, id string) {
	canceled, err := a.queue.cancel(r.Context(), id)
	if err != nil {
		http.Error(w, "Impossible d’annuler cet envoi.", http.StatusInternalServerError)
		return
	}
	if !canceled {
		http.Error(w, "L’envoi a déjà commencé ou a été annulé.", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) showPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Discussions []discussion
		Error       string
	}{}
	items, err := listDiscussions(r.Context(), a.root)
	if errors.Is(err, os.ErrNotExist) {
		data.Error = "Aucun historique Codex trouvé sur cet ordinateur."
	} else if err != nil {
		data.Error = "Impossible de lire l’historique local de Codex."
	} else {
		data.Discussions = items
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page.Execute(w, data)
}

func (a *app) download(w http.ResponseWriter, r *http.Request, id string) {
	item, err := findDiscussion(r.Context(), a.root, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data, err := exportDiscussion(r.Context(), a.root, item)
	if err != nil {
		http.Error(w, "Impossible d’exporter cette discussion. Vérifie que son historique est disponible sur cet ordinateur.",
			http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment",
		map[string]string{"filename": markdownFilename(item.Title)}))
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.Write(data)
}

func openBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("/usr/bin/open", address)
	case "windows":
		command = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe"),
			"url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	hideConsole(command)
	return command.Run()
}

func launchBrowser(address string, noBrowser bool) {
	fmt.Println(address)
	if !noBrowser {
		go func() {
			if err := openBrowser(address); err != nil {
				fmt.Fprintln(os.Stderr, "Ouvre l’adresse ci-dessus dans ton navigateur.")
			}
		}()
	}
}

func existingAddress(filename string) (string, error) {
	client := http.Client{
		Timeout:       350 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	for range 25 {
		data, err := os.ReadFile(filename)
		if err == nil {
			address := strings.TrimSpace(string(data))
			link, err := url.Parse(address)
			if err == nil && link.Scheme == "http" && link.Hostname() == "127.0.0.1" &&
				link.User == nil && len(link.Path) >= 28 && link.RawQuery == "" && link.Fragment == "" &&
				strings.HasSuffix(link.Path, "/") && strings.Count(link.Path, "/") == 2 {
				response, err := client.Get(address + "health")
				if err == nil {
					response.Body.Close()
					if response.StatusCode == http.StatusNoContent {
						return address, nil
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", errors.New("une autre instance de Celay est déjà ouverte")
}

func run(noBrowser bool) error {
	root, err := codexHome()
	if err != nil {
		return err
	}
	folder := os.Getenv("CELAY_DATA_DIR")
	if folder == "" {
		parent, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		folder = filepath.Join(parent, "Celay")
	}
	if err := os.MkdirAll(folder, 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(folder, "instance.lock"), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	addressFile := filepath.Join(folder, "instance.url")
	locked, err := lockFile(file)
	if err != nil {
		return err
	}
	if !locked {
		address, err := existingAddress(addressFile)
		if err != nil {
			return err
		}
		fmt.Println(address)
		if !noBrowser {
			return openBrowser(address)
		}
		return nil
	}
	queue, err := openQueue(folder)
	if err != nil {
		return err
	}
	defer queue.db.Close()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()

	a := &app{
		root: root, host: listener.Addr().String(), prefix: "/" + rand.Text() + "/",
		queue: queue, stop: make(chan struct{}, 1),
	}
	address := "http://" + a.host + a.prefix
	if err := os.WriteFile(addressFile, []byte(address), 0600); err != nil {
		return err
	}
	defer os.Remove(addressFile)
	ctx, cancel := context.WithCancel(context.Background())
	workersDone := make(chan struct{})
	go func() {
		defer close(workersDone)
		queue.run(ctx, func(ctx context.Context, item job, accepted func(string)) delivery {
			return sendToCodex(ctx, root, item, accepted)
		})
	}()
	defer func() { cancel(); <-workersDone }()
	a.lastSeen.Store(time.Now().Unix())
	server := &http.Server{
		Handler: a, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	launchBrowser(address, noBrowser)
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-tick.C:
			if time.Since(time.Unix(a.lastSeen.Load(), 0)) < 10*time.Minute {
				continue
			}
		case <-a.stop:
		}
		if active, err := queue.active(ctx); err != nil || active {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

func main() {
	noBrowser := flag.Bool("no-browser", false, "afficher l’adresse sans ouvrir le navigateur")
	flag.Parse()
	if err := run(*noBrowser); err != nil {
		fmt.Fprintln(os.Stderr, "Celay :", err)
		os.Exit(1)
	}
}
