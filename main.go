package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	irc "github.com/thoj/go-ircevent"
	"gopkg.in/yaml.v2"
)

type Network struct {
	Channels []string `yaml:"channels"`
	Server   string   `yaml:"server"`
	UseTLS   bool     `yaml:"usetls"`
	Port     int      `yaml:"port"`
}

type Config struct {
	Networks map[string]Network `yaml:"networks"`
	Nickname string             `yaml:"nickname"`
	Endpoint string             `yaml:"endpoint"`
	APIKey   string             `yaml:"apiKey"`
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

func (c *Config) Validate() error {
	if c.Nickname == "" {
		return fmt.Errorf("Nickname is missing from configuration")
	}
	if c.Endpoint == "" {
		return fmt.Errorf("Endpoint is missing from configuration")
	}
	if c.APIKey == "" {
		return fmt.Errorf("APIKey is missing from configuration")
	}
	for networkName, network := range c.Networks {
		if network.Server == "" {
			return fmt.Errorf("Server is missing from configuration for network: %s", networkName)
		}
		if len(network.Channels) == 0 {
			return fmt.Errorf("No channels specified in configuration for network: %s", networkName)
		}
		// More specific validations can be added here, such as checking the port range, TLS config, etc.
	}
	return nil
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

	body, err := io.ReadAll(resp.Body)
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
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("Error reading YAML file: %s\n", err)
	}

	// Unmarshal the YAML data into the Config struct
	err = yaml.Unmarshal([]byte(data), &config)
	if err != nil {
		log.Fatalf("Error parsing YAML file: %s\n", err)
	}

	err = config.Validate()
	if err != nil {
		log.Fatalf("Invalid configuration: %s\n", err)
	}

	var wg sync.WaitGroup

	for _, network := range config.Networks {
		wg.Add(1)

		go func(network Network) {
			defer wg.Done()

			// Create new IRC connection with nickname from config
			conn := irc.IRC(config.Nickname, config.Nickname)
			if conn == nil {
				log.Fatalf("Error creating IRC connection")
				return
			}

			conn.Debug = false
			conn.UseTLS = network.UseTLS
			conn.TLSConfig = &tls.Config{InsecureSkipVerify: true}

			// Add callback for IRC connection
			conn.AddCallback("001", func(e *irc.Event) {
				for _, channel := range network.Channels {
					conn.Join(channel)
				}
			})

			// Add callback for PRIVMSG
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
			server := fmt.Sprintf("%s:%d", network.Server, network.Port)
			err = conn.Connect(server)
			if err != nil {
				fmt.Printf("Err %s", err)
				return
			}

			// Start IRC connection
			conn.Loop()
		}(network)
	}

	wg.Wait()
}
