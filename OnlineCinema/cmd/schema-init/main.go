package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const subject = "movie-events-value"

func main() {
	registryURL := os.Getenv("SCHEMA_REGISTRY_URL")
	if registryURL == "" {
		log.Fatal("SCHEMA_REGISTRY_URL is not set")
	}
	schema, err := os.ReadFile("schemas/movie-event.avsc")
	if err != nil {
		log.Fatal("cannot read Avro schema: ", err)
	}
	schemaID, err := registerSchema(registryURL, string(schema))
	if err != nil {
		log.Fatal("cannot register Avro schema: ", err)
	}
	log.Printf("registered %s with schema ID %d", subject, schemaID)
}

func registerSchema(registryURL, schema string) (int, error) {
	body, err := json.Marshal(map[string]string{"schemaType": "AVRO", "schema": schema})
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	url := registryURL + "/subjects/" + subject + "/versions"
	resp, err := client.Post(url, "application/vnd.schemaregistry.v1+json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil {
			return 0, fmt.Errorf("registry returned %s: %w", resp.Status, err)
		}
		return 0, fmt.Errorf("registry returned %s: %s", resp.Status, detail)
	}
	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	return result.ID, nil
}
