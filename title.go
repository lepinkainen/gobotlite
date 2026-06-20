package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	irc "github.com/fluffle/goirc/client"
)

type TitlePayload struct {
	URL     string `json:"url"`
	Channel string `json:"channel"`
	User    string `json:"user"`
}

type TitleResponse struct {
	Title        string `json:"title"`
	ErrorMessage string `json:"errorMessage"`
}

// fetchLambdaTitle fetches the title using a Lambda function.
func fetchLambdaTitle(config *Config, payload *TitlePayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", config.LambdaTitle.Endpoint, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", config.LambdaTitle.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("Failed to close response body", "error", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var response TitleResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}

	if response.ErrorMessage != "" {
		return "", errors.New(response.ErrorMessage)
	}

	return response.Title, nil
}

// handleURL handles the URL received in the IRC event.
func handleURL(config *Config, conn *irc.Conn, line *irc.Line, urlStr string) {
	payload := &TitlePayload{
		URL:     urlStr,
		Channel: line.Args[0],
		User:    line.Src,
	}

	title, err := fetchLambdaTitle(config, payload)
	if err != nil {
		slog.Error("Error fetching Lambda title", "error", err, "url", urlStr)
		return
	}
	if title != "" {
		conn.Privmsg(line.Args[0], "Title: "+title)
	}
}
