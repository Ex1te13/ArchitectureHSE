package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(os.Args[1])
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	resp.Body.Close()
}
