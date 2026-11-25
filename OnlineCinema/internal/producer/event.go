package producer

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkedin/goavro/v2"
)

type Event struct {
	ID              string    `json:"event_id"`
	UserID          string    `json:"user_id"`
	MovieID         string    `json:"movie_id"`
	Type            string    `json:"event_type"`
	Timestamp       time.Time `json:"timestamp"`
	DeviceType      string    `json:"device_type"`
	SessionID       string    `json:"session_id"`
	ProgressSeconds *int32    `json:"progress_seconds"`
}

func (event Event) validate() error {
	if err := uuid.Validate(event.ID); len(event.ID) != 36 || err != nil {
		return fmt.Errorf("event_id must be a UUID")
	}
	if strings.TrimSpace(event.UserID) == "" || strings.TrimSpace(event.SessionID) == "" {
		return fmt.Errorf("user_id and session_id are required")
	}
	if strings.TrimSpace(event.MovieID) == "" && (event.Type != "SEARCHED" || event.MovieID != "") {
		return fmt.Errorf("movie_id is required except for searches without a specific movie")
	}
	if event.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	switch event.Type {
	case "VIEW_STARTED", "VIEW_PAUSED", "VIEW_RESUMED", "VIEW_FINISHED":
		if event.ProgressSeconds == nil || *event.ProgressSeconds < 0 {
			return fmt.Errorf("progress_seconds must be a non-negative integer for view events")
		}
	case "LIKED", "SEARCHED":
		if event.ProgressSeconds != nil {
			return fmt.Errorf("progress_seconds must be null for %s", event.Type)
		}
	}
	return nil
}

func (event Event) avroRecord() map[string]any {
	var progress any
	if event.ProgressSeconds != nil {
		progress = goavro.Union("int", *event.ProgressSeconds)
	}
	return map[string]any{
		"event_id":         event.ID,
		"user_id":          event.UserID,
		"movie_id":         event.MovieID,
		"event_type":       event.Type,
		"timestamp":        event.Timestamp,
		"device_type":      event.DeviceType,
		"session_id":       event.SessionID,
		"progress_seconds": progress,
	}
}
