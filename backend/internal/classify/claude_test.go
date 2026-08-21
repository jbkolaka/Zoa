package classify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// These tests exercise the real Anthropic SDK against a stand-in Messages API.
// They cannot check Claude's judgement, but they do check everything this
// codebase is actually responsible for: that the image is encoded and sent, that
// the response schema constrains the reply to the taxonomy, that a well-formed
// answer parses, and that every malformed or hostile answer degrades instead of
// reaching the database.
//
// ANTHROPIC_BASE_URL is how the SDK is redirected. Note that this also means a
// base URL set in the deployment environment silently reroutes production
// traffic — see the note in the phase notes.

// fakeAPI starts a stand-in Messages endpoint returning body, and captures the
// request it received.
func fakeAPI(t *testing.T, status int, body string) (*httptest.Server, *capturedRequest) {
	t.Helper()

	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		captured.path = r.URL.Path
		captured.body = raw
		_ = json.Unmarshal(raw, &captured.parsed)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return server, captured
}

type capturedRequest struct {
	path   string
	body   []byte
	parsed map[string]any
}

// messageJSON wraps a model reply in a Messages API response envelope.
func messageJSON(text string) string {
	payload, _ := json.Marshal(map[string]any{
		"id":          "msg_test",
		"type":        "message",
		"role":        "assistant",
		"model":       "claude-opus-5",
		"stop_reason": "end_turn",
		"content":     []map[string]any{{"type": "text", "text": text}},
		"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
	})
	return string(payload)
}

// newTestClassifier points a ClaudeClassifier at server.
func newTestClassifier(t *testing.T, server *httptest.Server) *ClaudeClassifier {
	t.Helper()
	t.Setenv("ANTHROPIC_BASE_URL", server.URL)
	return NewClaudeClassifier("test-key", "claude-opus-5")
}

func TestClaudeClassifierSendsImageAndSchema(t *testing.T) {
	server, captured := fakeAPI(t, http.StatusOK, messageJSON(
		`{"category":"pet","confidence":0.92,"alternatives":[{"category":"hdpe","confidence":0.04}]}`))

	classifier := newTestClassifier(t, server)
	image := []byte("\xff\xd8\xff\xe0 pretend jpeg bytes")

	got, err := classifier.Classify(context.Background(), Request{
		Image: image, MediaType: "image/jpeg", Filename: "whatever.jpg",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	// --- the parsed result ---
	if got.Category != "pet" {
		t.Errorf("category = %q, want pet", got.Category)
	}
	if got.Confidence != 0.92 {
		t.Errorf("confidence = %v, want 0.92", got.Confidence)
	}
	if len(got.Alternatives) != 1 || got.Alternatives[0].Category != "hdpe" {
		t.Errorf("alternatives = %+v, want [hdpe]", got.Alternatives)
	}

	// --- the outgoing request ---
	if !strings.HasSuffix(captured.path, "/v1/messages") {
		t.Errorf("path = %q, want it to end in /v1/messages", captured.path)
	}

	// The image must actually be in the payload, base64-encoded.
	encoded := base64.StdEncoding.EncodeToString(image)
	if !strings.Contains(string(captured.body), encoded) {
		t.Error("request body does not contain the base64-encoded image")
	}

	if model, _ := captured.parsed["model"].(string); model != "claude-opus-5" {
		t.Errorf("model = %v, want claude-opus-5", captured.parsed["model"])
	}

	// Effort low is what keeps the call inside the ~3s budget; if this regresses
	// to the default the endpoint starts degrading on latency instead.
	outputConfig, ok := captured.parsed["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config missing from request: %v", captured.parsed)
	}
	if effort, _ := outputConfig["effort"].(string); effort != "low" {
		t.Errorf("effort = %v, want low", outputConfig["effort"])
	}

	// The schema must pin the category to the taxonomy, or the model is free to
	// invent one and every response becomes a sanitise() rejection.
	format, ok := outputConfig["format"].(map[string]any)
	if !ok {
		t.Fatalf("output_config.format missing: %v", outputConfig)
	}
	if kind, _ := format["type"].(string); kind != "json_schema" {
		t.Errorf("format type = %v, want json_schema", format["type"])
	}
	if !strings.Contains(string(captured.body), `"garden_waste"`) {
		t.Error("request schema does not enumerate the taxonomy (no garden_waste in enum)")
	}
}

// A category outside the taxonomy must never reach the caller, even if the model
// somehow returns one despite the schema.
func TestClaudeClassifierRejectsOutOfTaxonomyReply(t *testing.T) {
	server, _ := fakeAPI(t, http.StatusOK, messageJSON(
		`{"category":"styrofoam_peanuts","confidence":0.99,"alternatives":[]}`))

	classifier := newTestClassifier(t, server)

	if _, err := classifier.Classify(context.Background(), Request{
		Image: []byte("x"), MediaType: "image/jpeg",
	}); err == nil {
		t.Fatal("expected an error for a category outside the taxonomy")
	}
}

func TestClaudeClassifierClampsConfidenceFromModel(t *testing.T) {
	server, _ := fakeAPI(t, http.StatusOK, messageJSON(
		`{"category":"pet","confidence":1.7,"alternatives":[]}`))

	classifier := newTestClassifier(t, server)

	got, err := classifier.Classify(context.Background(), Request{
		Image: []byte("x"), MediaType: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Confidence != 1 {
		t.Errorf("confidence = %v, want 1 (clamped)", got.Confidence)
	}
}

func TestClaudeClassifierHandlesUnparseableReply(t *testing.T) {
	for _, tc := range []struct{ name, reply string }{
		{"prose instead of json", "I think this is a plastic bottle."},
		{"truncated json", `{"category":"pet","confid`},
		{"empty text", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := fakeAPI(t, http.StatusOK, messageJSON(tc.reply))
			classifier := newTestClassifier(t, server)

			if _, err := classifier.Classify(context.Background(), Request{
				Image: []byte("x"), MediaType: "image/jpeg",
			}); err == nil {
				t.Error("expected an error, got a prediction")
			}
		})
	}
}

// A safety decline is a normal outcome that must surface as an error so the
// handler degrades to manual selection (FR-23).
func TestClaudeClassifierTreatsRefusalAsFailure(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"id":           "msg_test",
		"type":         "message",
		"role":         "assistant",
		"model":        "claude-opus-5",
		"stop_reason":  "refusal",
		"stop_details": map[string]any{"type": "refusal", "category": "cyber"},
		"content":      []map[string]any{},
		"usage":        map[string]any{"input_tokens": 10, "output_tokens": 0},
	})

	server, _ := fakeAPI(t, http.StatusOK, string(body))
	classifier := newTestClassifier(t, server)

	_, err := classifier.Classify(context.Background(), Request{
		Image: []byte("x"), MediaType: "image/jpeg",
	})
	if err == nil {
		t.Fatal("expected an error for a refusal")
	}
	if !strings.Contains(err.Error(), "declined") {
		t.Errorf("error = %v, want it to mention the decline", err)
	}
}

// An upstream 401 — exactly what a wrong key or a proxied base URL produces —
// must be an error the handler can degrade on, not a panic or a bogus result.
func TestClaudeClassifierHandlesUpstreamError(t *testing.T) {
	server, _ := fakeAPI(t, http.StatusUnauthorized,
		`{"error":{"message":"unauthorized"},"type":"unauthorized_error"}`)

	classifier := newTestClassifier(t, server)

	if _, err := classifier.Classify(context.Background(), Request{
		Image: []byte("x"), MediaType: "image/jpeg",
	}); err == nil {
		t.Fatal("expected an error for a 401")
	}
}

// The deadline is the caller's budget, so a slow upstream must be abandoned
// rather than allowed to stall the submission flow.
func TestClaudeClassifierRespectsContextDeadline(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(messageJSON(`{"category":"pet","confidence":0.9,"alternatives":[]}`)))
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()

	t.Setenv("ANTHROPIC_BASE_URL", slow.URL)
	classifier := NewClaudeClassifier("test-key", "claude-opus-5")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := classifier.Classify(ctx, Request{Image: []byte("x"), MediaType: "image/jpeg"})
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("expected a deadline error")
	}
	// Generous ceiling: the point is that it returns promptly, not that it hits an
	// exact number on a loaded CI box. The SDK may retry, so allow for that.
	if elapsed > 4*time.Second {
		t.Errorf("took %s to honour a 200ms deadline", elapsed.Round(time.Millisecond))
	}
}

func TestClaudeClassifierRejectsEmptyImage(t *testing.T) {
	server, _ := fakeAPI(t, http.StatusOK, messageJSON(`{"category":"pet","confidence":0.9,"alternatives":[]}`))
	classifier := newTestClassifier(t, server)

	if _, err := classifier.Classify(context.Background(), Request{MediaType: "image/jpeg"}); err == nil {
		t.Fatal("expected an error for an empty image")
	}
}

func TestClaudeClassifierNameReportsModel(t *testing.T) {
	classifier := NewClaudeClassifier("test-key", "claude-opus-5")
	if got, want := classifier.Name(), "claude:claude-opus-5"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

// An unset model must fall back to the documented default rather than sending an
// empty model id.
func TestClaudeClassifierDefaultsModel(t *testing.T) {
	classifier := NewClaudeClassifier("test-key", "")
	if !strings.Contains(classifier.Name(), DefaultModel) {
		t.Errorf("Name() = %q, want it to contain %q", classifier.Name(), DefaultModel)
	}
	if fmt.Sprint(DefaultModel) != "claude-opus-5" {
		t.Errorf("DefaultModel = %q, want claude-opus-5", DefaultModel)
	}
}
