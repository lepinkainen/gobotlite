package main

import (
	"errors"
	"fmt"
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

func (r CommandResponse) errMsg() string { return r.ErrorMessage }

// fetchLambdaCommand sends a POST request to a Lambda function endpoint with a given payload, and returns the result or an error.
func fetchLambdaCommand(config *Config, payload *CommandPayload) (string, error) {
	response, err := fetchLambda[CommandResponse](config.LambdaCommand, payload)
	if err != nil {
		return "", err
	}
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
	if len(command) == 0 {
		return errors.New("empty command string")
	}

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
	if len(fields) == 0 {
		return nil, nil
	}
	if len(fields) > 1 {
		return fields[:1], fields[1:]
	}
	return fields[:1], nil
}
