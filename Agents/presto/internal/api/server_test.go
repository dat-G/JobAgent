package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionsRunsLookupAndEvents(t *testing.T) {
	server := httptest.NewServer(NewServer(NewStore()))
	defer server.Close()

	session := postJSON[Session](t, server.URL+"/sessions", `{"metadata":{"owner":"api-test"}}`)
	if session.ID == "" {
		t.Fatal("expected session id")
	}

	run := postJSON[Run](t, server.URL+"/sessions/"+session.ID+"/runs", `{"input":{"prompt":"hello"}}`)
	if run.ID == "" {
		t.Fatal("expected run id")
	}
	if run.SessionID != session.ID {
		t.Fatalf("run session id = %q, want %q", run.SessionID, session.ID)
	}
	if run.Status != RunStatusQueued {
		t.Fatalf("run status = %q, want %q", run.Status, RunStatusQueued)
	}
	if run.EventCount != 1 {
		t.Fatalf("event count = %d, want 1", run.EventCount)
	}

	event := postJSON[Event](t, server.URL+"/runs/"+run.ID+"/events", `{"type":"token","data":{"text":"world"}}`)
	if event.ID == "" {
		t.Fatal("expected event id")
	}
	if event.RunID != run.ID {
		t.Fatalf("event run id = %q, want %q", event.RunID, run.ID)
	}

	lookedUp := getJSON[Run](t, server.URL+"/runs/"+run.ID)
	if lookedUp.EventCount != 2 {
		t.Fatalf("looked up event count = %d, want 2", lookedUp.EventCount)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/runs/"+run.ID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}

	frame := readSSEFrame(t, resp.Body)
	if !strings.Contains(frame, "event: run.created") {
		t.Fatalf("first frame = %q, want run.created event", frame)
	}
}

func TestRunStatusPatch(t *testing.T) {
	server := httptest.NewServer(NewServer(NewStore()))
	defer server.Close()

	session := postJSON[Session](t, server.URL+"/sessions", `{}`)
	run := postJSON[Run](t, server.URL+"/sessions/"+session.ID+"/runs", `{}`)

	req, err := http.NewRequest(http.MethodPatch, server.URL+"/runs/"+run.ID, strings.NewReader(`{"status":"completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var updated Run
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status != RunStatusCompleted {
		t.Fatalf("updated status = %q, want %q", updated.Status, RunStatusCompleted)
	}
	if updated.FinishedAt == nil {
		t.Fatal("expected finished_at after completed status")
	}
	if updated.EventCount != 2 {
		t.Fatalf("event count = %d, want 2", updated.EventCount)
	}
}

func TestUnknownRunEventsReturnNotFound(t *testing.T) {
	server := httptest.NewServer(NewServer(NewStore()))
	defer server.Close()

	resp, err := http.Get(server.URL + "/runs/missing/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func postJSON[T any](t *testing.T, url, body string) T {
	t.Helper()

	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST %s status = %d, want %d", url, resp.StatusCode, http.StatusCreated)
	}

	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func getJSON[T any](t *testing.T, url string) T {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, http.StatusOK)
	}

	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func readSSEFrame(t *testing.T, body ioReader) string {
	t.Helper()

	reader := bufio.NewReader(body)
	var frame bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\n" {
			return frame.String()
		}
		frame.WriteString(line)
	}
}

type ioReader interface {
	Read([]byte) (int, error)
}
