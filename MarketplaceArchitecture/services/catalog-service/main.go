package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(healthResponse{
		Status:  "ok",
		Service: "catalog-service",
	})
}

func main() {
	http.HandleFunc("/health", health)
	log.Fatal(http.ListenAndServe(":8000", nil))
}
