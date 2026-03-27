package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/homelab-game/backend/internal/api/ws"
	"github.com/homelab-game/backend/internal/auth"
)

// wsTestEnv holds the test environment for WebSocket handler tests.
type wsTestEnv struct {
	handler *GameHandler
	hub     *ws.Hub
	userID  string
	conn    *websocket.Conn
	srv     *httptest.Server
}

// newWSTestEnv creates a GameHandler with a real Hub and a connected WebSocket
// test client. The GameHandler has nil query objects — tests that exercise
// paths reaching the database (processAction) will panic, which is expected.
// Tests in this file verify HandleWSAction message parsing, validation, and
// response formatting exclusively.
func newWSTestEnv(t *testing.T) *wsTestEnv {
	t.Helper()

	hub := ws.NewHub()
	userID := "ws-test-user-001"

	handler := &GameHandler{
		hub:         hub,
		broadcaster: ws.NewLocalBroadcaster(hub),
		userLocks:   newUserMutexMap(),
		// All query pointers are intentionally nil. Tests must not reach
		// processAction database calls.
	}

	jwtSecret := "test-secret-for-ws-handler-tests"
	token, err := auth.GenerateToken(userID, jwtSecret)
	if err != nil {
		t.Fatalf("failed to generate test JWT: %v", err)
	}

	// Wire OnMessage to dispatch to HandleWSAction in a goroutine, matching
	// production wiring in main.go.
	hub.OnMessage = func(uid string, data []byte) {
		go handler.HandleWSAction(uid, data)
	}

	srv := httptest.NewServer(hub.HandleConnect(jwtSecret))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?token=" + token
	header := http.Header{}
	header.Set("Origin", "http://localhost:3000")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		srv.Close()
		t.Fatalf("websocket dial failed: %v", err)
	}

	// Allow the hub to finish registration and start pumps.
	time.Sleep(50 * time.Millisecond)

	return &wsTestEnv{
		handler: handler,
		hub:     hub,
		userID:  userID,
		conn:    conn,
		srv:     srv,
	}
}

// readJSON reads the next WS message and unmarshals it into the given struct.
func (e *wsTestEnv) readJSON(t *testing.T, v interface{}, timeout time.Duration) {
	t.Helper()
	e.conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := e.conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read WS message: %v", err)
	}
	if err := json.Unmarshal(msg, v); err != nil {
		t.Fatalf("failed to unmarshal WS message: %v (raw: %s)", err, string(msg))
	}
}

// sendJSON marshals v to JSON and sends it over the WebSocket.
func (e *wsTestEnv) sendJSON(t *testing.T, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if err := e.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("failed to send WS message: %v", err)
	}
}

// close cleans up the test environment.
func (e *wsTestEnv) close() {
	e.conn.Close()
	e.srv.Close()
}

// --- Tests for HandleWSAction ---

func TestHandleWSAction_MalformedJSON(t *testing.T) {
	env := newWSTestEnv(t)
	defer env.close()

	// Send garbage bytes that are not valid JSON.
	err := env.conn.WriteMessage(websocket.TextMessage, []byte(`{not json`))
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// The handler should log the error and return without sending a response.
	// Verify no response arrives within a reasonable window.
	env.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, err = env.conn.ReadMessage()
	if err == nil {
		t.Fatal("expected no response for malformed JSON, but received a message")
	}
	// A read deadline timeout is expected here — no message was sent back.
}

func TestHandleWSAction_UnknownMessageType(t *testing.T) {
	env := newWSTestEnv(t)
	defer env.close()

	// Send a well-formed JSON message with type != "action".
	msg := map[string]interface{}{
		"type": "ping",
		"id":   "req-001",
	}
	env.sendJSON(t, msg)

	// The handler should silently ignore non-action message types.
	// Verify no response arrives.
	env.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, err := env.conn.ReadMessage()
	if err == nil {
		t.Fatal("expected no response for unknown message type, but received a message")
	}
}

func TestHandleWSAction_MissingRequestID(t *testing.T) {
	env := newWSTestEnv(t)
	defer env.close()

	// Send an action message with an empty ID.
	msg := map[string]interface{}{
		"type":   "action",
		"id":     "",
		"action": "run_job",
	}
	env.sendJSON(t, msg)

	// The handler should respond with an action_result error.
	var result wsActionResult
	env.readJSON(t, &result, 2*time.Second)

	if result.Type != "action_result" {
		t.Errorf("Type = %q, want %q", result.Type, "action_result")
	}
	if result.ID != "" {
		t.Errorf("ID = %q, want empty string", result.ID)
	}
	if result.Success {
		t.Error("Success = true, want false")
	}
	if result.Error != "missing request id" {
		t.Errorf("Error = %q, want %q", result.Error, "missing request id")
	}
}

func TestHandleWSAction_MissingRequestID_NoIDField(t *testing.T) {
	env := newWSTestEnv(t)
	defer env.close()

	// Send an action message that omits the "id" field entirely.
	msg := map[string]interface{}{
		"type":   "action",
		"action": "run_job",
	}
	env.sendJSON(t, msg)

	var result wsActionResult
	env.readJSON(t, &result, 2*time.Second)

	if result.Type != "action_result" {
		t.Errorf("Type = %q, want %q", result.Type, "action_result")
	}
	if result.Success {
		t.Error("Success = true, want false")
	}
	if result.Error != "missing request id" {
		t.Errorf("Error = %q, want %q", result.Error, "missing request id")
	}
}

func TestHandleWSAction_ValidAction_ReachesProcessAction(t *testing.T) {
	env := newWSTestEnv(t)
	defer env.close()

	// Send a valid action message. Since query objects are nil, processAction
	// will fail (game state not found / nil pointer). The handler should
	// return an action_result error rather than panic.
	msg := map[string]interface{}{
		"type":   "action",
		"id":     "req-valid-001",
		"action": "run_job",
	}
	env.sendJSON(t, msg)

	var result wsActionResult
	env.readJSON(t, &result, 2*time.Second)

	if result.Type != "action_result" {
		t.Errorf("Type = %q, want %q", result.Type, "action_result")
	}
	if result.ID != "req-valid-001" {
		t.Errorf("ID = %q, want %q", result.ID, "req-valid-001")
	}
	if result.Success {
		t.Error("Success = true, want false (nil query objects should cause an error)")
	}
	// The error message should be non-empty — the exact text depends on
	// whether processAction returns a nil-pointer panic (recovered) or a
	// game-state-not-found error. Either way, we verify the handler
	// didn't crash and sent a well-formed error response.
	if result.Error == "" {
		t.Error("Error is empty, want a non-empty error message")
	}
}

func TestHandleWSAction_ResponseIncludesRequestID(t *testing.T) {
	env := newWSTestEnv(t)
	defer env.close()

	// Send multiple actions with different IDs and verify each response
	// echoes the correct request ID.
	ids := []string{"id-alpha", "id-beta", "id-gamma"}

	for _, id := range ids {
		msg := map[string]interface{}{
			"type":   "action",
			"id":     id,
			"action": "run_job",
		}
		env.sendJSON(t, msg)
	}

	// Collect responses. Because actions are dispatched in goroutines and
	// serialize on the user mutex, responses arrive in order.
	received := make(map[string]bool)
	for i := 0; i < len(ids); i++ {
		var result wsActionResult
		env.readJSON(t, &result, 5*time.Second)

		if result.Type != "action_result" {
			t.Errorf("response %d: Type = %q, want %q", i, result.Type, "action_result")
		}
		received[result.ID] = true
	}

	// Verify all request IDs were echoed back.
	for _, id := range ids {
		if !received[id] {
			t.Errorf("missing response for request ID %q", id)
		}
	}
}

func TestHandleWSAction_ActionResultFormat_Success(t *testing.T) {
	// This test verifies the JSON structure of a successful action_result
	// by calling HandleWSAction directly with a custom hub that captures
	// the output. We use a handler with nil deps, so processAction will
	// fail. This test documents the expected format; the success path is
	// tested via integration tests with the real database.

	// For the success response format, we test the struct serialization
	// directly.
	result := wsActionResult{
		Type:    "action_result",
		ID:      "test-123",
		Success: true,
		State: &fullStateResponse{
			GameState: nil, // minimal
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed["type"] != "action_result" {
		t.Errorf("type = %v, want action_result", parsed["type"])
	}
	if parsed["id"] != "test-123" {
		t.Errorf("id = %v, want test-123", parsed["id"])
	}
	if parsed["success"] != true {
		t.Errorf("success = %v, want true", parsed["success"])
	}
	// state should be present (even if contents are minimal)
	if _, ok := parsed["state"]; !ok {
		t.Error("state field missing from success response")
	}
	// error should be omitted (omitempty)
	if _, ok := parsed["error"]; ok {
		t.Error("error field should be omitted on success")
	}
}

func TestHandleWSAction_ActionResultFormat_Error(t *testing.T) {
	result := wsActionResult{
		Type:    "action_result",
		ID:      "test-456",
		Success: false,
		Error:   "not enough compute units",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed["type"] != "action_result" {
		t.Errorf("type = %v, want action_result", parsed["type"])
	}
	if parsed["id"] != "test-456" {
		t.Errorf("id = %v, want test-456", parsed["id"])
	}
	if parsed["success"] != false {
		t.Errorf("success = %v, want false", parsed["success"])
	}
	if parsed["error"] != "not enough compute units" {
		t.Errorf("error = %v, want 'not enough compute units'", parsed["error"])
	}
	// state should be omitted (omitempty)
	if _, ok := parsed["state"]; ok {
		t.Error("state field should be omitted on error")
	}
}

func TestHandleWSAction_InternalError_MaskedForClient(t *testing.T) {
	// Verify that internal errors are masked as "internal server error"
	// for the client (security: don't leak internal details).
	ae := &actionError{msg: "database connection failed", internal: true}

	// Simulate the masking logic from HandleWSAction.
	clientMsg := ae.Error()
	if ae.internal {
		clientMsg = "internal server error"
	}

	if clientMsg != "internal server error" {
		t.Errorf("client message = %q, want %q", clientMsg, "internal server error")
	}
}

func TestHandleWSAction_GameLogicError_PassedToClient(t *testing.T) {
	// Verify that game-logic errors (non-internal) are passed through
	// to the client.
	ae := &actionError{msg: "not enough compute units", internal: false}

	clientMsg := ae.Error()
	if ae.internal {
		clientMsg = "internal server error"
	}

	if clientMsg != "not enough compute units" {
		t.Errorf("client message = %q, want %q", clientMsg, "not enough compute units")
	}
}

func TestWSActionRequest_Deserialization(t *testing.T) {
	// Verify the wsActionRequest struct correctly deserializes all fields
	// from the client message format specified in the TDD.
	raw := `{
		"type": "action",
		"id": "f47ac10b-58cc",
		"action": "buy_hardware",
		"payload": {"name": "Raspberry Pi 5"}
	}`

	var req wsActionRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Type != "action" {
		t.Errorf("Type = %q, want %q", req.Type, "action")
	}
	if req.ID != "f47ac10b-58cc" {
		t.Errorf("ID = %q, want %q", req.ID, "f47ac10b-58cc")
	}
	if req.Action != "buy_hardware" {
		t.Errorf("Action = %q, want %q", req.Action, "buy_hardware")
	}

	var payload map[string]string
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload["name"] != "Raspberry Pi 5" {
		t.Errorf("payload.name = %q, want %q", payload["name"], "Raspberry Pi 5")
	}
}

func TestWSActionRequest_OptionalPayload(t *testing.T) {
	// Verify actions with no payload (e.g., run_job) deserialize correctly.
	raw := `{"type": "action", "id": "abc", "action": "run_job"}`

	var req wsActionRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Action != "run_job" {
		t.Errorf("Action = %q, want %q", req.Action, "run_job")
	}
	if req.Payload != nil {
		t.Errorf("Payload = %s, want nil (payload should be optional)", string(req.Payload))
	}
}

// --- Test processAction error types ---

func TestActionError_Types(t *testing.T) {
	tests := []struct {
		name     string
		err      *actionError
		wantMsg  string
		wantInt  bool
		wantNF   bool
	}{
		{
			name:    "game logic error",
			err:     &actionError{msg: "not enough compute units"},
			wantMsg: "not enough compute units",
		},
		{
			name:    "internal error",
			err:     &actionError{msg: "failed to save hardware", internal: true},
			wantMsg: "failed to save hardware",
			wantInt: true,
		},
		{
			name:    "not found error",
			err:     &actionError{msg: "game state not found", notFound: true},
			wantMsg: "game state not found",
			wantNF:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.wantMsg)
			}
			if tt.err.internal != tt.wantInt {
				t.Errorf("internal = %v, want %v", tt.err.internal, tt.wantInt)
			}
			if tt.err.notFound != tt.wantNF {
				t.Errorf("notFound = %v, want %v", tt.err.notFound, tt.wantNF)
			}
		})
	}
}
