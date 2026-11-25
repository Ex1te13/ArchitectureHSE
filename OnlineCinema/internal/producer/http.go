package producer

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (p *Producer) ServeEvent(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var event Event
	if err := decoder.Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expected one JSON event"})
		return
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	event.Timestamp = event.Timestamp.UTC().Truncate(time.Millisecond)
	value, err := p.encode(event)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := p.publish(r.Context(), event, value); err != nil {
		slog.Error("cannot publish event", "event_id", event.ID, "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cannot publish event"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"event_id": event.ID})
}

func writeJSON(w http.ResponseWriter, status int, body map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
