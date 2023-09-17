package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	irc "github.com/thoj/go-ircevent"
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

func fetchLambdaCommand(config *Config, payload *CommandPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	fmt.Printf("Calling lambda command with payload %s\n", data)

	req, err := http.NewRequest("POST", config.LambdaCommand.Endpoint, bytes.NewBuffer(data))
	if err != nil {
		fmt.Println("error constructing request")
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", config.LambdaCommand.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("error doing request")
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error reading response body")
		return "", err
	}

	var response CommandResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		fmt.Println("unmarshaling response")
		return "", err
	}

	// for debug
	//fmt.Println("response", response)

	if response.ErrorMessage != "" {
		return "", errors.New(response.ErrorMessage)
	}

	return response.Result, nil
}

func handleCommand(config *Config, conn *irc.Connection, e *irc.Event, commandStr string) {

	var command []string
	var args []string

	// Don't break if there's not enough to split
	fields := strings.Fields(commandStr)
	if len(fields) > 1 {
		args = fields[1:]
		command = fields[:1]
	} else {
		command = fields[:1]
	}

	payload := &CommandPayload{
		Command: strings.Join(command, " "),
		Args:    strings.Join(args, " "),
		Channel: e.Arguments[0],
		User:    e.Source,
	}

	response, err := fetchLambdaCommand(config, payload)

	if err != nil {
		log.Printf("Error handling lambda command: %s", err)
		return
	}
	if response != "" {
		conn.Privmsg(e.Arguments[0], response)
	}
}
