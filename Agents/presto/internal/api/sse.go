package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) streamRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	lastEventID := r.Header.Get("Last-Event-ID")
	if after := r.URL.Query().Get("after"); after != "" {
		lastEventID = after
	}

	replay, events, unsubscribe, ok := s.store.Subscribe(runID, lastEventID)
	if !ok {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	for _, event := range replay {
		if err := writeSSE(w, event); err != nil {
			return
		}
		flusher.Flush()
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-events:
			if err := writeSSE(w, event); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeSSE(w io.Writer, event Event) error {
	data := compactJSON(event.Data)
	if event.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", sseField(event.ID)); err != nil {
			return err
		}
	}
	if event.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", sseField(event.Type)); err != nil {
			return err
		}
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func compactJSON(data json.RawMessage) []byte {
	if len(data) == 0 {
		return []byte("{}")
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return []byte("{}")
	}
	return compact.Bytes()
}

func sseField(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n':
			return -1
		default:
			return r
		}
	}, value)
}
