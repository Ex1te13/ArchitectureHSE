package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type event struct {
	ID              string    `json:"event_id"`
	UserID          string    `json:"user_id"`
	MovieID         string    `json:"movie_id"`
	Type            string    `json:"event_type"`
	Timestamp       time.Time `json:"timestamp"`
	DeviceType      string    `json:"device_type"`
	SessionID       string    `json:"session_id"`
	ProgressSeconds *int32    `json:"progress_seconds"`
}

func TestPipeline(t *testing.T) {
	producerURL := os.Getenv("PRODUCER_URL")
	clickhouseURL := os.Getenv("CLICKHOUSE_URL")
	if producerURL == "" && clickhouseURL == "" {
		t.Skip("set PRODUCER_URL and CLICKHOUSE_URL to run the pipeline test")
	}
	if producerURL == "" || clickhouseURL == "" {
		t.Fatal("PRODUCER_URL and CLICKHOUSE_URL are required")
	}
	client := &http.Client{Timeout: 25 * time.Second}
	endpoint := producerURL + "/events"
	waitForProducer(t, client, endpoint)

	runID := uuid.NewString()
	progress := int32(120)
	for _, input := range []event{
		{Type: "VIEW_PAUSED", MovieID: "test-movie", ProgressSeconds: &progress},
		{Type: "SEARCHED"},
	} {
		t.Run(input.Type, func(t *testing.T) {
			input.ID = uuid.NewString()
			input.UserID = "test-user-" + runID
			input.SessionID = "test-session-" + runID
			input.DeviceType = "TV"
			input.Timestamp = time.Now().UTC().Truncate(time.Millisecond)

			publish(t, client, endpoint, input)
			got := waitForEvent(t, client, clickhouseURL, input)
			if !reflect.DeepEqual(got, input) {
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(input)
				t.Fatalf("got %s\nwant %s", gotJSON, wantJSON)
			}
		})
	}
}

func waitForProducer(t *testing.T, client *http.Client, endpoint string) {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		resp, err := client.Get(endpoint)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusMethodNotAllowed {
				return
			}
		}
	}
	t.Fatal("producer did not start listening on " + endpoint)
}

func publish(t *testing.T, client *http.Client, endpoint string, input event) {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		detail, _ := io.ReadAll(resp.Body)
		t.Fatalf("producer returned %s: %s", resp.Status, detail)
	}
	var result struct {
		ID string `json:"event_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.ID != input.ID {
		t.Fatalf("producer returned event ID %q, want %q", result.ID, input.ID)
	}
}

func waitForEvent(t *testing.T, client *http.Client, endpoint string, input event) event {
	t.Helper()
	query := `SELECT event_id, user_id, movie_id, event_type, timestamp, device_type, session_id, progress_seconds
FROM cinema.movie_events
WHERE toDate(timestamp) = {day:Date} AND event_id = {id:UUID}
LIMIT 1 FORMAT JSONEachRow`
	params := url.Values{
		"param_day":               {input.Timestamp.Format(time.DateOnly)},
		"param_id":                {input.ID},
		"date_time_output_format": {"iso"},
	}
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		req, err := http.NewRequest(http.MethodPost, endpoint+"/?"+params.Encode(), strings.NewReader(query))
		if err != nil {
			t.Fatal(err)
		}
		req.SetBasicAuth(os.Getenv("CLICKHOUSE_USER"), os.Getenv("CLICKHOUSE_PASSWORD"))
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			detail, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("ClickHouse returned %s: %s", resp.Status, detail)
		}
		var got event
		err = json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		if err == nil {
			return got
		}
		if !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
	}
	t.Fatalf("event %s did not reach ClickHouse", input.ID)
	return event{}
}
