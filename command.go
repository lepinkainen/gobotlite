package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	irc "github.com/fluffle/goirc/client"
)

type CommandPayload struct {
	Command string `json:"command"`
	Args    string `json:"args"`
	Channel string `json:"channel"`
	User    string `json:"user"`
}

type CommandResponse struct {
	Result       string `json:"result"`
	ErrorMessage string `json:"errorMessage"`
}

// fetchLambdaCommand sends a POST request to a Lambda function endpoint with a given payload, and returns the result or an error.
func fetchLambdaCommand(config *Config, payload *CommandPayload) (string, error) {
	// Marshal the payload struct into JSON format
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	slog.Debug("Calling lambda command", "payload", string(data))

	// Construct the HTTP request
	req, err := http.NewRequest("POST", config.LambdaCommand.Endpoint, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("error constructing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", config.LambdaCommand.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error doing request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("Failed to close response body", "error", err)
		}
	}()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	// Unmarshal the response body into a CommandResponse struct
	var response CommandResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", fmt.Errorf("error unmarshaling response: %w", err)
	}

	// Check if the response has an error message
	if response.ErrorMessage != "" {
		return "", errors.New(response.ErrorMessage)
	}

	// Return the result and nil error
	return response.Result, nil
}

// handleCommand handles an IRC command by sending it to a Lambda function for processing and sending the response back to the IRC connection.
// It takes in a `Config` struct pointer, an IRC connection pointer, an IRC event pointer, and a string representing the command as arguments.
func handleCommand(config *Config, conn *irc.Conn, line *irc.Line, commandStr string) error {
	// Validate input
	if commandStr == "" {
		return errors.New("empty command string")
	}

	// Split command string into command and arguments
	command, args := splitCommandString(commandStr)

	// Create a CommandPayload struct with the command, arguments, channel, and user information
	payload := &CommandPayload{
		Command: strings.Join(command, " "),
		Args:    strings.Join(args, " "),
		Channel: line.Args[0],
		User:    line.Src,
	}

	// Call the fetchLambdaCommand function to send the payload to the Lambda function and get the response
	response, err := fetchLambdaCommand(config, payload)
	if err != nil {
		return fmt.Errorf("error handling lambda command: %w", err)
	}

	if response != "" {
		// Send the response back to the IRC connection
		conn.Privmsg(line.Args[0], response)
	}

	return nil
}

// splitCommandString splits a command string into command and arguments.
func splitCommandString(commandStr string) ([]string, []string) {
	fields := strings.Fields(commandStr)
	if len(fields) > 1 {
		return fields[:1], fields[1:]
	}
	return fields[:1], nil
}
