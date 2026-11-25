package producer

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
)

func (p *Producer) Generate(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, event := range generateSession() {
				value, err := p.encode(event)
				if err == nil {
					err = p.publish(ctx, event, value)
				}
				if err != nil {
					slog.Error("cannot publish generated event", "event_id", event.ID, "error", err)
					break
				}
			}
		}
	}
}

func generateSession() []Event {
	devices := []string{"MOBILE", "DESKTOP", "TV", "TABLET"}
	device := devices[rand.IntN(len(devices))]
	user := fmt.Sprintf("demo-user-%d", 1+rand.IntN(20))
	movie := fmt.Sprintf("demo-movie-%d", 1+rand.IntN(10))
	session := uuid.NewString()
	finished := int32(3600 + rand.IntN(3601))
	paused := finished / 3
	zero := int32(0)
	start := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Duration(finished+120) * time.Second)
	createEvent := func(eventType, movieID string, seconds int32, progress *int32) Event {
		return Event{
			ID:              uuid.NewString(),
			UserID:          user,
			MovieID:         movieID,
			Type:            eventType,
			Timestamp:       start.Add(time.Duration(seconds) * time.Second),
			DeviceType:      device,
			SessionID:       session,
			ProgressSeconds: progress,
		}
	}
	events := []Event{
		createEvent("SEARCHED", "", 0, nil),
		createEvent("VIEW_STARTED", movie, 10, &zero),
		createEvent("VIEW_PAUSED", movie, paused+10, &paused),
	}
	if rand.IntN(4) == 0 {
		return events
	}
	events = append(events,
		createEvent("VIEW_RESUMED", movie, paused+40, &paused),
		createEvent("VIEW_FINISHED", movie, finished+40, &finished),
	)
	if rand.IntN(3) == 0 {
		events = append(events, createEvent("LIKED", movie, finished+50, nil))
	}
	return events
}
