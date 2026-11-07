package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	_ "modernc.org/sqlite"
)

type discussion struct {
	ID, Title, Rollout, Mode string
	Archived, HasMessages    bool
}

func codexHome() (string, error) {
	if folder := strings.TrimSpace(os.Getenv("CODEX_HOME")); folder != "" {
		return filepath.Abs(folder)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func latestDatabase(root, prefix string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	bestVersion, bestName := -1, ""
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix+"_") || !strings.HasSuffix(name, ".sqlite") {
			continue
		}
		version, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, prefix+"_"), ".sqlite"))
		if err == nil && version > bestVersion {
			bestVersion, bestName = version, name
		}
	}
	if bestName == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(root, bestName), nil
}

func openDatabase(root, prefix string) (*sql.DB, error) {
	path, err := latestDatabase(root, prefix)
	if err != nil {
		return nil, err
	}
	path = filepath.ToSlash(path)
	if runtime.GOOS == "windows" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	dsn := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&_pragma=busy_timeout(1500)"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func listDiscussions(ctx context.Context, root string) ([]discussion, error) {
	db, err := openDatabase(root, "state")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT name FROM pragma_table_info('threads')")
	if err != nil {
		return nil, err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			rows.Close()
			return nil, err
		}
		columns[column] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	name, mode, source, recency := "", "'legacy'", "", "updated_at"
	if columns["name"] {
		name = "NULLIF(TRIM(name), ''), "
	}
	if columns["history_mode"] {
		mode = "history_mode"
	}
	if columns["thread_source"] {
		source = " AND COALESCE(thread_source, 'user') = 'user'"
	}
	if columns["recency_at_ms"] {
		recency = "COALESCE(NULLIF(recency_at_ms, 0), updated_at * 1000)"
	}
	hasMessages := "has_user_event = 1"
	if columns["first_user_message"] {
		hasMessages += " OR first_user_message <> ''"
	}
	if columns["preview"] {
		hasMessages += " OR preview <> ''"
	}
	rows, err = db.QueryContext(ctx,
		"SELECT id, COALESCE("+name+"NULLIF(TRIM(title), ''), 'Discussion sans titre'), "+
			"rollout_path, archived, "+mode+", CASE WHEN "+hasMessages+" THEN 1 ELSE 0 END FROM threads "+
			"WHERE source NOT LIKE '%subagent%'"+source+" ORDER BY "+recency+" DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []discussion
	for rows.Next() {
		var item discussion
		if err := rows.Scan(&item.ID, &item.Title, &item.Rollout, &item.Archived, &item.Mode, &item.HasMessages); err != nil {
			return nil, err
		}
		item.Title = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(item.Title))
		if item.Title == "" {
			item.Title = "Discussion sans titre"
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func findDiscussion(ctx context.Context, root, id string) (discussion, error) {
	items, err := listDiscussions(ctx, root)
	if err != nil {
		return discussion{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return discussion{}, sql.ErrNoRows
}

type contentPart struct {
	Type string
	Text string
}

type message struct {
	Type, Text, Message, Phase string
	Content                    []contentPart
	Images                     []json.RawMessage
}

func (m message) appendMarkdown(markdown *bytes.Buffer) bool {
	var heading string
	switch m.Type {
	case "userMessage", "UserMessage", "user_message":
		heading = "Vous"
	case "agentMessage", "AgentMessage", "agent_message":
		heading = "Codex"
		if m.Phase == "commentary" {
			heading = "Codex · Suivi"
		}
	default:
		return false
	}
	var parts []string
	addText := func(text string) {
		if strings.TrimSpace(text) != "" {
			parts = append(parts, strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
		}
	}
	addText(m.Text)
	addText(m.Message)
	for _, part := range m.Content {
		switch {
		case strings.EqualFold(part.Type, "text"):
			addText(part.Text)
		case strings.Contains(strings.ToLower(part.Type), "image"):
			parts = append(parts, "_[Image jointe]_")
		case part.Type != "":
			parts = append(parts, "_[Contenu joint]_")
		}
	}
	if len(m.Images) > 0 {
		parts = append(parts, "_[Image jointe]_")
	}
	if len(parts) == 0 {
		return false
	}
	fmt.Fprintf(markdown, "\n## %s\n\n%s\n", heading, strings.TrimRight(strings.Join(parts, "\n\n"), "\n"))
	return true
}

func exportDiscussion(ctx context.Context, root string, item discussion) ([]byte, error) {
	var markdown bytes.Buffer
	title := item.Title
	for _, char := range "\\`*_{}[]()#+.!<>|~-" {
		if char == '\\' {
			title = strings.ReplaceAll(title, "\\", "\\\\")
		} else {
			title = strings.ReplaceAll(title, string(char), "\\"+string(char))
		}
	}
	fmt.Fprintln(&markdown, "# "+title)
	found := false
	if item.Mode != "legacy" {
		db, err := openDatabase(root, "thread_history")
		if err == nil {
			defer db.Close()
			rows, err := db.QueryContext(ctx, "SELECT item_json FROM thread_items WHERE thread_id = ? AND "+
				"item_type IN ('userMessage', 'agentMessage') ORDER BY rollout_ordinal", item.ID)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			for rows.Next() {
				var data []byte
				if err := rows.Scan(&data); err != nil {
					return nil, err
				}
				var entry message
				if err := json.Unmarshal(data, &entry); err != nil {
					return nil, err
				}
				if entry.appendMarkdown(&markdown) {
					found = true
				}
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if !found && item.Rollout != "" {
		var err error
		found, err = appendLegacy(ctx, root, item.Rollout, &markdown)
		if err != nil {
			return nil, err
		}
	}
	if !found {
		if item.HasMessages {
			return nil, errors.New("messages introuvables dans l’historique local")
		}
		fmt.Fprintln(&markdown, "\n_Aucun message enregistré._")
	}
	return markdown.Bytes(), nil
}

func appendLegacy(ctx context.Context, root, path string, markdown *bytes.Buffer) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		file, err = os.Open(filepath.Join(root, "archived_sessions", filepath.Base(path)))
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 64<<20)
	found := false
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		line := scanner.Bytes()
		if !bytes.Contains(line, []byte(`"event_msg"`)) {
			continue
		}
		var row struct {
			Type    string
			Payload json.RawMessage
		}
		if json.Unmarshal(line, &row) != nil || row.Type != "event_msg" {
			continue
		}
		var event struct {
			Type string
			Item json.RawMessage
		}
		if json.Unmarshal(row.Payload, &event) != nil {
			continue
		}
		payload := row.Payload
		if event.Type == "item_completed" {
			payload = event.Item
		}
		switch event.Type {
		case "item_completed", "user_message", "agent_message":
			var entry message
			if json.Unmarshal(payload, &entry) == nil && entry.appendMarkdown(markdown) {
				found = true
			}
		}
	}
	return found, scanner.Err()
}

func markdownFilename(title string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return ' '
		}
		return r
	}, title)
	name = strings.Trim(strings.Join(strings.Fields(name), " "), ". ")
	if len(name) > 160 {
		for index := range name {
			if index > 156 {
				name = strings.TrimRight(name[:index], ". ")
				break
			}
		}
	}
	if name == "" {
		name = "Discussion Codex"
	}
	return name + ".md"
}
