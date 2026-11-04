//go:build integration

package main

import (
	"io"
	"net/http"
	"testing"
)

func TestIntegrationHealthReady(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	response, err := env.server.Client().Get(
		env.server.URL + "/health/ready",
	)
	if err != nil {
		t.Fatalf("Failed to send health check request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)

		t.Fatalf(
			"Expected HTTP status %d, got %d. Response: %s",
			http.StatusOK,
			response.StatusCode,
			string(body),
		)
	}
}
