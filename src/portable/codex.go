package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var errCodexMissing = errors.New("Codex introuvable. Installe Codex ou indique le chemin de son exécutable avec CELAY_CODEX.")

func locateCodex(ctx context.Context) (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CELAY_CODEX")); custom != "" {
		if filepath.IsAbs(custom) && isCodexExecutable(custom) {
			return custom, nil
		}
		return "", errors.New("CELAY_CODEX doit indiquer le chemin absolu d’un exécutable Codex.")
	}
	program := "codex"
	if runtime.GOOS == "windows" {
		program = "codex.exe"
	}
	if path, err := exec.LookPath(program); err == nil && filepath.IsAbs(path) {
		return path, nil
	}
	home, _ := os.UserHomeDir()
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			for _, target := range []string{"x64", "arm64"} {
				root := filepath.Join(appData, "npm", "node_modules", "@openai", "codex")
				vendor := filepath.Join(root, "node_modules", "@openai", "codex-win32-"+target, "vendor")
				matches, _ := filepath.Glob(filepath.Join(vendor, "*", "codex", program))
				candidates = append(candidates, matches...)
				matches, _ = filepath.Glob(filepath.Join(vendor, "*", "bin", program))
				candidates = append(candidates, matches...)
			}
		}
	case "darwin":
		candidates = append(candidates,
			"/Applications/Codex.app/Contents/Resources/codex",
			filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex"),
			"/opt/homebrew/bin/codex", "/usr/local/bin/codex")
	default:
		candidates = append(candidates, "/usr/local/bin/codex", "/usr/bin/codex")
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", program))
	}
	for _, path := range candidates {
		if isCodexExecutable(path) {
			return path, nil
		}
	}
	if path := packagedCodex(ctx); path != "" && isCodexExecutable(path) {
		return path, nil
	}
	return "", errCodexMissing
}

func isCodexExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Ext(path), ".exe")
	}
	return info.Mode()&0111 != 0
}

type rpcError struct {
	Code    int
	Message string
}

func (err *rpcError) Error() string { return err.Message }

type rpcPacket struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
	Result json.RawMessage
	Error  *rpcError
}

type rpcInput struct {
	packet rpcPacket
	err    error
}

type codexProcess struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	write *json.Encoder
	read  chan rpcInput
	stop  chan struct{}
	next  int
	early []json.RawMessage
}

func startCodex(ctx context.Context, binary string) (*codexProcess, error) {
	cmd := exec.CommandContext(ctx, binary, "app-server", "--listen", "stdio://")
	hideConsole(cmd)
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	client := &codexProcess{
		cmd: cmd, stdin: input, write: json.NewEncoder(input),
		read: make(chan rpcInput), stop: make(chan struct{}),
	}
	go func() {
		decode := json.NewDecoder(output)
		for {
			var packet rpcPacket
			err := decode.Decode(&packet)
			select {
			case client.read <- rpcInput{packet, err}:
			case <-client.stop:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return client, nil
}

func (client *codexProcess) close() {
	close(client.stop)
	client.stdin.Close()
	done := make(chan struct{})
	go func() {
		client.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		client.cmd.Process.Kill()
		<-done
	}
}

func (client *codexProcess) receive(ctx context.Context) (rpcPacket, error) {
	for {
		select {
		case <-ctx.Done():
			return rpcPacket{}, ctx.Err()
		case input := <-client.read:
			packet := input.packet
			if input.err == nil && packet.Method != "" && len(packet.ID) > 0 {
				if err := client.answerRequest(ctx, packet); err != nil {
					return rpcPacket{}, err
				}
				continue
			}
			return packet, input.err
		}
	}
}

func (client *codexProcess) writePacket(ctx context.Context, packet any) error {
	done := make(chan error, 1)
	go func() { done <- client.write.Encode(packet) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		client.cmd.Process.Kill()
		<-done
		return ctx.Err()
	}
}

func (client *codexProcess) answerRequest(ctx context.Context, packet rpcPacket) error {
	response := map[string]any{"id": packet.ID}
	switch packet.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval",
		"execCommandApproval", "applyPatchApproval":
		response["result"] = map[string]string{"decision": "decline"}
	case "item/permissions/requestApproval":
		response["result"] = map[string]any{"permissions": map[string]any{}, "scope": "turn"}
	case "item/tool/requestUserInput":
		response["result"] = map[string]any{"answers": map[string]any{}}
	case "mcpServer/elicitation/request":
		response["result"] = map[string]any{"action": "decline", "content": nil}
	default:
		response["error"] = map[string]any{"code": -32601, "message": "Celay cannot handle this interactive request"}
	}
	return client.writePacket(ctx, response)
}

func (client *codexProcess) call(ctx context.Context, method string, params, result any) error {
	client.next++
	id := client.next
	if err := client.writePacket(ctx, map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		packet, err := client.receive(ctx)
		if err != nil {
			return err
		}
		if packet.Method == "turn/completed" && len(client.early) < 8 {
			client.early = append(client.early, packet.Params)
		}
		if packet.Method != "" || string(packet.ID) != strconv.Itoa(id) {
			continue
		}
		if packet.Error != nil {
			return packet.Error
		}
		return json.Unmarshal(packet.Result, result)
	}
}

type codexTurn struct {
	ID, Status string
	Error      *struct{ Message string }
}

func isBusy(err error) bool {
	var rpc *rpcError
	if !errors.As(err, &rpc) {
		return false
	}
	message := strings.ToLower(rpc.Message)
	for _, phrase := range []string{
		"has an active turn", "already has an active writer", "already running", "already in use", "already locked",
		"lock held", "locked by another", "holding the writer lock", "writer lease",
	} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

func busyDelivery() delivery {
	return delivery{state: "retry", detail: "Discussion occupée. Celay réessaiera automatiquement."}
}

func sendToCodex(ctx context.Context, root string, item job, accepted func(string)) delivery {
	preflight, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	thread, err := findDiscussion(preflight, root, item.ThreadID)
	if err != nil {
		return delivery{state: "failed", detail: "Discussion introuvable dans l’historique local de Codex."}
	}
	if thread.Archived {
		return delivery{state: "failed", detail: "Désarchive la discussion dans Codex pour pouvoir y envoyer un message."}
	}
	binary, err := locateCodex(preflight)
	if err != nil {
		return delivery{state: "failed", detail: err.Error()}
	}
	client, err := startCodex(ctx, binary)
	if err != nil {
		return delivery{state: "failed", detail: "Impossible de démarrer Codex. " + err.Error()}
	}
	defer client.close()

	var result json.RawMessage
	err = client.call(preflight, "initialize", map[string]any{
		"clientInfo": map[string]string{"name": "celay", "title": "Celay", "version": version},
	}, &result)
	if err != nil {
		return delivery{state: "failed", detail: "Initialisation de Codex impossible. " + err.Error()}
	}
	if err := client.writePacket(preflight, map[string]string{"method": "initialized"}); err != nil {
		return delivery{state: "failed", detail: "Connexion à Codex interrompue."}
	}
	var resumed struct {
		Thread struct {
			ID     string
			Status struct{ Type string }
		}
	}
	err = client.call(preflight, "thread/resume",
		map[string]any{"threadId": item.ThreadID, "excludeTurns": true}, &resumed)
	if err != nil {
		if isBusy(err) {
			return busyDelivery()
		}
		return delivery{state: "failed", detail: "Impossible de reprendre la discussion. " + err.Error()}
	}
	if resumed.Thread.ID != item.ThreadID {
		return delivery{state: "failed", detail: "Codex a retourné une autre discussion. Envoi annulé."}
	}
	if resumed.Thread.Status.Type == "active" {
		return busyDelivery()
	}
	cancel()
	start, stop := context.WithTimeout(ctx, 60*time.Second)
	var started struct{ Turn codexTurn }
	err = client.call(start, "turn/start", map[string]any{
		"threadId": item.ThreadID,
		"input":    []map[string]string{{"type": "text", "text": item.Prompt}},
	}, &started)
	stop()
	if err != nil {
		var rpc *rpcError
		if errors.As(err, &rpc) {
			if isBusy(err) {
				return busyDelivery()
			}
			return delivery{state: "failed", detail: "Codex a refusé l’envoi. " + err.Error()}
		}
		return delivery{state: "uncertain", detail: "Connexion interrompue pendant l’envoi. Vérifie la discussion avant de reprogrammer."}
	}
	turn := started.Turn
	if turn.ID == "" {
		return delivery{state: "uncertain", detail: "Codex n’a pas confirmé le tour. Vérifie la discussion avant de reprogrammer."}
	}
	accepted(turn.ID)
	if turn.Status != "inProgress" && turn.Status != "" {
		return completedTurn(turn)
	}
	for _, event := range client.early {
		if finished, ok := matchingTurn(event, item.ThreadID, turn.ID); ok {
			return completedTurn(finished)
		}
	}
	for {
		packet, err := client.receive(ctx)
		if err != nil {
			return delivery{state: "uncertain", turnID: turn.ID,
				detail: "Codex a accepté le message, mais Celay a perdu la connexion. Vérifie la réponse dans la discussion."}
		}
		if packet.Method != "turn/completed" {
			continue
		}
		if finished, ok := matchingTurn(packet.Params, item.ThreadID, turn.ID); ok {
			return completedTurn(finished)
		}
	}
}

func matchingTurn(params json.RawMessage, threadID, turnID string) (codexTurn, bool) {
	var done struct {
		ThreadID string
		Turn     codexTurn
	}
	err := json.Unmarshal(params, &done)
	return done.Turn, err == nil && done.Turn.ID == turnID &&
		(done.ThreadID == "" || done.ThreadID == threadID)
}

func completedTurn(turn codexTurn) delivery {
	if turn.Status == "completed" {
		return delivery{state: "sent", turnID: turn.ID}
	}
	detail := "Le message a été accepté, mais Codex n’a pas terminé sa réponse."
	if turn.Error != nil && turn.Error.Message != "" {
		detail = fmt.Sprintf("%s %s", detail, turn.Error.Message)
	}
	return delivery{state: "failed", detail: detail, turnID: turn.ID}
}
