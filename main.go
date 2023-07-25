package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"

	irc "github.com/thoj/go-ircevent"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Server   []string `yaml:"server"`
	Channels []string `yaml:"channels"`
	Nick     string   `yaml:"nick"`
	Endpoint string   `yaml:"endpoint"`
	APIKey   string   `yaml:"apiKey"`
}

type Payload struct {
	URL     string `json:"url"`
	Channel string `json:"channel"`
	User    string `json:"user"`
}

type HTTPResponse struct {
	Title        string `json:"title"`
	ErrorMessage string `json:"errorMessage"`
}

func fetchLambdaTitle(config *Config, payload *Payload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", config.Endpoint, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", config.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var response HTTPResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}

	if response.ErrorMessage != "" {
		return "", errors.New(response.ErrorMessage)
	}

	return response.Title, nil
}

func handleURL(config *Config, conn *irc.Connection, e *irc.Event, urlStr string) {
	payload := &Payload{
		URL:     urlStr,
		Channel: e.Arguments[0],
		User:    e.Nick,
	}

	title, err := fetchLambdaTitle(config, payload)
	if err != nil {
		log.Printf("Error fetching Lambda title: %s", err)
		return
	}
	if title != "" {
		conn.Privmsg(e.Arguments[0], "Title: "+title)
	}
}

func main() {
	config := Config{}

	// Read the YAML configuration file
	data, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("Error reading YAML file: %s\n", err)
	}

	// Unmarshal the YAML data into the Config struct
	err = yaml.Unmarshal([]byte(data), &config)
	if err != nil {
		log.Fatalf("Error parsing YAML file: %s\n", err)
	}

	// Create new IRC connection with nickname from config
	conn := irc.IRC(config.Nick, config.Nick)
	conn.Debug = true

	// Add callback for IRC connection
	conn.AddCallback("001", func(e *irc.Event) {
		for _, channel := range config.Channels {
			conn.Join(channel)
		}
	})

	conn.AddCallback("PRIVMSG", func(e *irc.Event) {
		words := strings.Fields(e.Message())
		for _, word := range words {
			u, err := url.Parse(word)
			if err != nil {
				log.Printf("Error parsing potential URL '%s': %s", word, err)
			} else if u.Scheme != "" && u.Host != "" {
				// Valid URL detected, handle accordingly
				log.Printf("URL detected: %s", u.String())
				go handleURL(&config, conn, e, u.String())
			}
		}
	})

	// Add callback for PING messages
	conn.AddCallback("PING", func(e *irc.Event) { conn.SendRaw("PONG :" + e.Message()) })

	// Connect to the IRC server
	err = conn.Connect(config.Server[0])
	if err != nil {
		fmt.Printf("Err %s", err)
		return
	}

	// Start IRC connection
	conn.Loop()
}