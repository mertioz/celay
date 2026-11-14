package main

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func readDesktopTestPacket(conn net.Conn) (map[string]any, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > 8<<20 {
		return nil, errors.New("invalid test frame")
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}
	var packet map[string]any
	if err := json.Unmarshal(payload, &packet); err != nil {
		return nil, err
	}
	return packet, nil
}

func TestDesktopDeliveryUsesExistingOwner(t *testing.T) {
	for _, scenario := range []string{"idle", "active", "lost-ack"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			db, err := sql.Open("sqlite", filepath.Join(root, "thread_history_1.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			_, err = db.Exec("CREATE TABLE thread_turns(thread_id TEXT, turn_id TEXT, rollout_ordinal INTEGER, status TEXT, error_json TEXT)")
			if err != nil {
				t.Fatal(err)
			}
			state := "completed"
			if scenario == "active" {
				state = "inProgress"
			}
			if _, err := db.Exec("INSERT INTO thread_turns VALUES('target','previous',0,?,NULL)", state); err != nil {
				t.Fatal(err)
			}
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()
			client := &desktopClient{conn: clientConn}
			server := &desktopClient{conn: serverConn}
			requests := make(chan map[string]any, 8)
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					packet, err := readDesktopTestPacket(serverConn)
					if err != nil {
						return
					}
					requests <- packet
					var result any
					switch packet["method"] {
					case "initialize":
						result = map[string]string{"clientId": "celay-client"}
					case "thread-owner-discovery":
						result = map[string]bool{"supportsUntrustedAppInput": true}
					case "thread-follower-read-model-settings":
						result = map[string]any{"settings": map[string]string{"model": "saved-model"}}
					case "thread-follower-start-turn":
						if scenario == "lost-ack" {
							serverConn.Close()
							return
						}
						if _, err := db.Exec("INSERT INTO thread_turns VALUES('target','accepted',1,'completed',NULL)"); err != nil {
							return
						}
						result = map[string]any{"result": map[string]any{
							"turn": map[string]string{"id": "accepted", "status": "inProgress"},
						}}
					}
					if err := server.write(map[string]any{
						"type": "response", "requestId": packet["requestId"],
						"resultType": "success", "method": packet["method"],
						"handledByClientId": "desktop-owner", "result": result,
					}); err != nil {
						return
					}
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := client.initialize(ctx, "target"); err != nil {
				t.Fatal(err)
			}
			var accepted []string
			result := client.deliver(ctx, root,
				job{ID: "scheduled", ThreadID: "target", Prompt: "Only once"},
				func(id string) { accepted = append(accepted, id) })
			expected := map[string]string{"idle": "sent", "active": "retry", "lost-ack": "uncertain"}[scenario]
			if result.state != expected {
				t.Fatalf("got %+v", result)
			}
			clientConn.Close()
			<-done
			close(requests)
			mutations := 0
			for packet := range requests {
				if packet["method"] != "thread-follower-start-turn" {
					continue
				}
				mutations++
				if packet["version"] != float64(2) || packet["targetClientId"] != "desktop-owner" {
					t.Fatalf("message was not routed to the discovered owner: %+v", packet)
				}
				params := packet["params"].(map[string]any)
				start := params["turnStart"].(map[string]any)
				request := start["request"].(map[string]any)
				if params["conversationId"] != "target" || request["threadId"] != "target" ||
					start["context"].(map[string]any)["inheritThreadSettings"] != true {
					t.Fatalf("unexpected routed request: %+v", packet)
				}
				messageID, validID := request["clientUserMessageId"].(string)
				if _, err := uuid.Parse(messageID); err != nil || !validID {
					t.Fatalf("Desktop messages require a UUID client identifier: %+v", request)
				}
				input := request["input"].([]any)[0].(map[string]any)
				elements, valid := input["text_elements"].([]any)
				if !valid || len(elements) != 0 || input["text"] != "Only once" {
					t.Fatalf("Desktop text inputs require the renderer's metadata array: %+v", input)
				}
				if _, set := request["model"]; set {
					t.Fatal("default delivery must inherit the owner's model")
				}
			}
			if scenario == "active" && mutations != 0 {
				t.Fatal("an active response received a second message")
			}
			if scenario != "active" && mutations != 1 {
				t.Fatalf("expected exactly one submission, got %d", mutations)
			}
			if scenario == "idle" && (len(accepted) != 1 || accepted[0] != "accepted") {
				t.Fatalf("accepted turn was not recorded: %v", accepted)
			}
			if scenario == "lost-ack" && len(accepted) != 0 {
				t.Fatal("a lost acknowledgment must stay uncertain")
			}
		})
	}
}

func TestTurnStartParamsPreserveSelectedModelAndReasoning(t *testing.T) {
	params := turnStartParams(job{ThreadID: "target", Prompt: "Message"}, turnOptions{Model: "selected", Effort: "high"})
	if params["model"] != "selected" || params["effort"] != "high" || params["threadId"] != "target" {
		t.Fatalf("selected options were lost: %+v", params)
	}
	// Mirror the renderer's metadata lookup across the serialized wire input.
	wire, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Input []struct {
			TextElements json.RawMessage `json:"text_elements"`
		}
	}
	if err := json.Unmarshal(wire, &decoded); err != nil || len(decoded.Input) != 1 ||
		string(decoded.Input[0].TextElements) != "[]" {
		t.Fatalf("text metadata must survive JSON serialization as an empty array: %s, %v", wire, err)
	}
}

func TestInstalledDesktopOwnerReadOnly(t *testing.T) {
	threadID := os.Getenv("CELAY_DESKTOP_PROBE_THREAD")
	if threadID == "" {
		t.Skip("set CELAY_DESKTOP_PROBE_THREAD to inspect an existing owner without sending a message")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root, err := codexHome()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := dialDesktop(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := &desktopClient{conn: conn}
	if err := client.initialize(ctx, threadID); err != nil {
		t.Fatal(err)
	}
	var result struct{ Settings *struct{ Model string } }
	if err := client.call(ctx, "thread-follower-read-model-settings", 1,
		map[string]string{"conversationId": threadID}, &result); err != nil {
		t.Fatal(err)
	}
	if result.Settings == nil || result.Settings.Model == "" {
		t.Fatal("the existing owner did not return its settings")
	}
}
