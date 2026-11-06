package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

//go:embed page.html
var pageSource string

var page = template.Must(template.New("index").Parse(pageSource))

type app struct {
	root, host, prefix string
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
	case r.Method == http.MethodPost && path == "alive":
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && path == "close":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html lang="fr"><meta charset="utf-8"><title>Codex Markdown</title><p>Codex Markdown est fermé. Vous pouvez fermer cet onglet.</p></html>`)
		select {
		case a.stop <- struct{}{}:
		default:
		}
	default:
		http.NotFound(w, r)
	}
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
	items, err := listDiscussions(r.Context(), a.root)
	if err != nil {
		http.Error(w, "Impossible de lire l’historique local de Codex.", http.StatusServiceUnavailable)
		return
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		data, err := exportDiscussion(r.Context(), a.root, item)
		if err != nil {
			http.Error(w, "Impossible d’exporter cette discussion. Vérifiez que son historique est disponible sur cet ordinateur.",
				http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment",
			map[string]string{"filename": markdownFilename(item.Title)}))
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		w.Write(data)
		return
	}
	http.NotFound(w, r)
}

func openBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("/usr/bin/open", address)
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	return command.Run()
}

func run(noBrowser bool) error {
	root, err := codexHome()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()

	a := &app{
		root: root, host: listener.Addr().String(), prefix: "/" + rand.Text() + "/",
		stop: make(chan struct{}, 1),
	}
	a.lastSeen.Store(time.Now().Unix())
	server := &http.Server{
		Handler: a, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	address := "http://" + a.host + a.prefix
	fmt.Println(address)
	if !noBrowser {
		go func() {
			if err := openBrowser(address); err != nil {
				fmt.Fprintln(os.Stderr, "Ouvrez l’adresse ci-dessus dans votre navigateur.")
			}
		}()
	}
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

func main() {
	noBrowser := flag.Bool("no-browser", false, "afficher l’adresse sans ouvrir le navigateur")
	flag.Parse()
	if err := run(*noBrowser); err != nil {
		fmt.Fprintln(os.Stderr, "Codex Markdown :", err)
		os.Exit(1)
	}
}
