package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// lambdaResponse is implemented by Lambda response payloads so fetchLambda can
// surface an application-level error returned in the body.
type lambdaResponse interface {
	errMsg() string
}

// fetchLambda POSTs payload as JSON to the given Lambda endpoint and decodes the
// response into T. It returns an error for transport failures, undecodable
// responses, or a non-empty errorMessage in the response body.
func fetchLambda[T lambdaResponse](api APIConfig, payload any) (T, error) {
	var response T

	data, err := json.Marshal(payload)
	if err != nil {
		return response, err
	}

	slog.Debug("Calling lambda", "endpoint", api.Endpoint, "payload", string(data))

	req, err := http.NewRequest("POST", api.Endpoint, bytes.NewBuffer(data))
	if err != nil {
		return response, fmt.Errorf("error constructing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", api.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return response, fmt.Errorf("error doing request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("Failed to close response body", "error", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return response, fmt.Errorf("error reading response body: %w", err)
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return response, fmt.Errorf("error unmarshaling response: %w", err)
	}

	if response.errMsg() != "" {
		return response, errors.New(response.errMsg())
	}

	return response, nil
}
