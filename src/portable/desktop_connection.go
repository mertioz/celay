package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/google/uuid"
)

// Codex Desktop's local coordination protocol routes follower requests to
// the client holding the writer lock. The owner checks the request versions.
type desktopClient struct {
	conn     net.Conn
	clientID string
	ownerID  string
}

type desktopPacket struct {
	Type              string          `json:"type"`
	RequestID         string          `json:"requestId"`
	Method            string          `json:"method"`
	ResultType        string          `json:"resultType"`
	Error             string          `json:"error"`
	HandledByClientID string          `json:"handledByClientId"`
	Result            json.RawMessage `json:"result"`
}

type desktopError string

func (err desktopError) Error() string { return string(err) }

func (client *desktopClient) write(packet any) error {
	payload, err := json.Marshal(packet)
	if err != nil {
		return err
	}
	frame := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(frame, uint32(len(payload)))
	copy(frame[4:], payload)
	_, err = io.Copy(client.conn, bytes.NewReader(frame))
	return err
}

func (client *desktopClient) call(ctx context.Context, method string, version int, params any, result any) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	if err := client.conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { client.conn.Close() })
	defer stop()
	requestID := rand.Text()
	if err := client.write(map[string]any{
		"type": "request", "requestId": requestID, "sourceClientId": client.clientID,
		"method": method, "version": version, "params": params, "timeoutMs": 10000,
		"targetClientId": client.ownerID,
	}); err != nil {
		return err
	}
	for {
		var header [4]byte
		if _, err := io.ReadFull(client.conn, header[:]); err != nil {
			return err
		}
		length := binary.LittleEndian.Uint32(header[:])
		if length == 0 || length > 8<<20 {
			return errors.New("réponse locale Codex invalide")
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(client.conn, payload); err != nil {
			return err
		}
		var packet desktopPacket
		if err := json.Unmarshal(payload, &packet); err != nil {
			return err
		}
		if packet.Type == "client-discovery-request" {
			if err := client.write(map[string]any{
				"type": "client-discovery-response", "requestId": packet.RequestID,
				"response": map[string]bool{"canHandle": false},
			}); err != nil {
				return err
			}
		}
		if packet.Type != "response" || packet.RequestID != requestID {
			continue
		}
		if packet.ResultType != "success" {
			return desktopError(packet.Error)
		}
		if method == "thread-owner-discovery" {
			client.ownerID = packet.HandledByClientID
		}
		return json.Unmarshal(packet.Result, result)
	}
}

func (client *desktopClient) initialize(ctx context.Context, threadID string) error {
	var initialized struct{ ClientID string }
	if err := client.call(ctx, "initialize", 0, map[string]string{"clientType": "celay"}, &initialized); err != nil {
		return err
	}
	if initialized.ClientID == "" {
		return errors.New("identifiant local Codex manquant")
	}
	client.clientID = initialized.ClientID
	var owner json.RawMessage
	if err := client.call(ctx, "thread-owner-discovery", 1,
		map[string]string{"hostId": "local", "conversationId": threadID}, &owner); err != nil {
		return err
	}
	if client.ownerID == "" {
		return errors.New("propriétaire de discussion introuvable")
	}
	return nil
}

func tryDesktopDelivery(ctx context.Context, root string, item job, accepted func(string)) (delivery, bool) {
	if os.Getenv("CELAY_CODEX") != "" {
		return delivery{}, false
	}
	connect, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := dialDesktop(connect, root)
	if err != nil {
		return delivery{}, false
	}
	defer conn.Close()
	client := &desktopClient{conn: conn}
	if err := client.initialize(connect, item.ThreadID); err != nil {
		return delivery{}, false
	}
	return client.deliver(ctx, root, item, accepted), true
}

func (client *desktopClient) deliver(ctx context.Context, root string, item job, accepted func(string)) delivery {
	preflight, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var settings struct {
		Settings *struct{ Model string }
	}
	err := client.call(preflight, "thread-follower-read-model-settings", 1,
		map[string]string{"conversationId": item.ThreadID}, &settings)
	if err != nil || settings.Settings == nil || settings.Settings.Model == "" {
		return delivery{state: "failed", detail: "Impossible de lire les réglages de la discussion dans Codex Desktop."}
	}
	db, err := openDatabase(root, "thread_history")
	if err != nil {
		return delivery{state: "failed", detail: "Impossible de lire l’état de la discussion avant l’envoi."}
	}
	defer db.Close()
	var state string
	err = db.QueryRowContext(preflight,
		"SELECT status FROM thread_turns WHERE thread_id=? ORDER BY rollout_ordinal DESC LIMIT 1", item.ThreadID).Scan(&state)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return delivery{state: "failed", detail: "Impossible de vérifier si Codex répond déjà dans cette discussion."}
	}
	if state == "inProgress" {
		return busyDelivery()
	}
	options := item.Options
	if options.Model != "" {
		models, err := loadModelCatalog(preflight)
		if err != nil {
			return delivery{state: "failed", detail: err.Error()}
		}
		options, err = validateTurnOptions(models, options)
		if err != nil {
			return delivery{state: "failed", detail: err.Error()}
		}
	}
	params := turnStartParams(item, options)
	params["clientUserMessageId"] = uuid.NewSHA1(uuid.NameSpaceURL, []byte("celay:"+item.ID)).String()
	var response struct{ Result struct{ Turn codexTurn } }
	err = client.call(preflight, "thread-follower-start-turn", 2, map[string]any{
		"conversationId": item.ThreadID,
		"turnStart":      map[string]any{"request": params, "context": map[string]bool{"inheritThreadSettings": true}},
	}, &response)
	if err != nil {
		fmt.Println("Celay : réponse d’envoi de Codex Desktop non confirmée.", err)
		var rejected desktopError
		if errors.As(err, &rejected) && isBusy(&rpcError{Message: rejected.Error()}) {
			return busyDelivery()
		}
		return delivery{state: "uncertain",
			detail: "Codex Desktop n’a pas confirmé l’envoi. Vérifie la discussion avant de reprogrammer."}
	}
	turn := response.Result.Turn
	if turn.ID == "" {
		return delivery{state: "uncertain", detail: "Codex Desktop n’a pas retourné l’identifiant de l’envoi. Vérifie la discussion."}
	}
	accepted(turn.ID)
	if turn.Status != "" && turn.Status != "inProgress" {
		return completedTurn(turn)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastCheck := time.Now()
	for {
		var errorJSON sql.NullString
		err := db.QueryRowContext(ctx, "SELECT status, error_json FROM thread_turns WHERE thread_id=? AND turn_id=?",
			item.ThreadID, turn.ID).Scan(&turn.Status, &errorJSON)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return delivery{state: "uncertain", turnID: turn.ID, detail: "Message accepté par Codex Desktop. Vérifie sa réponse dans la discussion."}
		}
		if err == nil && turn.Status != "inProgress" {
			if errorJSON.Valid {
				json.Unmarshal([]byte(errorJSON.String), &turn.Error)
			}
			return completedTurn(turn)
		}
		if time.Since(lastCheck) >= 10*time.Second {
			check, stop := context.WithTimeout(ctx, 3*time.Second)
			err := client.call(check, "thread-follower-read-model-settings", 1,
				map[string]string{"conversationId": item.ThreadID}, &settings)
			stop()
			if err != nil {
				return delivery{state: "uncertain", turnID: turn.ID,
					detail: "Message accepté, puis connexion à Codex Desktop perdue. Vérifie la réponse dans la discussion."}
			}
			lastCheck = time.Now()
		}
		select {
		case <-ctx.Done():
			return delivery{state: "uncertain", turnID: turn.ID, detail: "Message accepté par Codex Desktop. Vérifie sa réponse dans la discussion."}
		case <-ticker.C:
		}
	}
}

func turnStartParams(item job, options turnOptions) map[string]any {
	params := map[string]any{
		"threadId": item.ThreadID,
		// Desktop renders this input optimistically before the server normalizes
		// it. Its renderer requires text_elements to be an array, including when empty.
		"input": []map[string]any{{
			"type": "text", "text": item.Prompt, "text_elements": []any{},
		}},
	}
	if options.Model != "" {
		params["model"] = options.Model
	}
	if options.Effort != "" {
		params["effort"] = options.Effort
	}
	return params
}

func writerBusyDelivery() delivery {
	return delivery{state: "retry", detail: "Cette discussion reste ouverte dans un autre moteur Codex. " +
		"Ferme cette discussion dans cet autre moteur pour libérer l’envoi."}
}
