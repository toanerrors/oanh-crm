package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://127.0.0.1:8080/api/v1/session")
	if err != nil {
		os.Exit(1)
	}
	defer response.Body.Close()
	var session struct {
		NeedsSetup *bool `json:"needsSetup"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&session) != nil || session.NeedsSetup == nil {
		os.Exit(1)
	}
}
