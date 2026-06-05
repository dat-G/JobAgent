package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPAPIListsRunsForSession(t *testing.T) {
	server := httptest.NewServer(NewServer(NewStore()))
	defer server.Close()

	session := postJSON[Session](t, server.URL+"/sessions", `{"metadata":{"owner":"api-test"}}`)
	first := postJSON[Run](t, server.URL+"/sessions/"+session.ID+"/runs", `{"input":{"prompt":"first"}}`)
	second := postJSON[Run](t, server.URL+"/sessions/"+session.ID+"/runs", `{"input":{"prompt":"second"}}`)

	listed := getJSON[struct {
		Runs []Run `json:"runs"`
	}](t, server.URL+"/sessions/"+session.ID+"/runs")

	if len(listed.Runs) != 2 {
		t.Fatalf("listed runs = %d, want 2", len(listed.Runs))
	}
	if listed.Runs[0].ID != first.ID || listed.Runs[1].ID != second.ID {
		t.Fatalf("runs should be listed in creation order: %#v", listed.Runs)
	}
}

func TestHTTPAPIRejectsInvalidJSONAndUnknownFields(t *testing.T) {
	server := httptest.NewServer(NewServer(NewStore()))
	defer server.Close()

	resp, err := http.Post(server.URL+"/sessions", "application/json", strings.NewReader(`{"metadata":`))
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp, http.StatusBadRequest)

	resp, err = http.Post(server.URL+"/sessions", "application/json", strings.NewReader(`{"metadata":{},"unexpected":true}`))
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestHTTPAPIMethodValidation(t *testing.T) {
	server := httptest.NewServer(NewServer(NewStore()))
	defer server.Close()

	resp, err := http.Post(server.URL+"/healthz", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	if allow := resp.Header.Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow header = %q, want %q", allow, http.MethodGet)
	}
}

func assertStatus(t *testing.T, resp *http.Response, status int) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != status {
		t.Fatalf("status = %d, want %d", resp.StatusCode, status)
	}
}
