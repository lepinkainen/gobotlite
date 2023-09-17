package main

import (
	"crypto/tls"
	"fmt"
	"log"
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

type APIConfig struct {
	Endpoint string `yaml:"endpoint"`
	APIKey   string `yaml:"apiKey"`
}

type Config struct {
	Networks      map[string]Network `yaml:"networks"`
	Nickname      string             `yaml:"nickname"`
	LambdaTitle   APIConfig          `yaml:"lambdatitle"`
	LambdaCommand APIConfig          `yaml:"lambdacommand"`
	Addit         APIConfig          `yaml:"addconfig"`
}

func (c *Config) Validate() error {
	if c.Nickname == "" {
		return fmt.Errorf("nickname is missing from configuration")
	}
	if c.LambdaCommand.Endpoint == "" {
		return fmt.Errorf("command endpoint is missing from configuration")
	}
	if c.LambdaTitle.Endpoint == "" {
		return fmt.Errorf("title endpoint is missing from configuration")
	}
	for networkName, network := range c.Networks {
		if network.Server == "" {
			return fmt.Errorf("server is missing from configuration for network: %s", networkName)
		}
		if len(network.Channels) == 0 {
			return fmt.Errorf("no channels specified in configuration for network: %s", networkName)
		}
		// More specific validations can be added here, such as checking the port range, TLS config, etc.
	}
	return nil
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
					// Default to #channels
					if !strings.HasPrefix(channel, "#") {
						channel = "#" + channel
					}
					conn.Join(channel)
				}
			})

			conn.AddCallback("366", func(e *irc.Event) {
				log.Printf("Joined %s", e.Arguments[1])
			})

			// Add callback for PRIVMSG
			conn.AddCallback("PRIVMSG", func(e *irc.Event) {
				// Ignore other bots
				if e.Nick == "Sinkko" {
					return
				}

				// log.Printf("PRIVMSG: %s", e.Message())

				words := strings.Fields(e.Message())

				// nothing to process
				if len(words) == 0 {
					return
				}

				// Handle rexpl as a special case
				if words[0] == ".rexpl" && (e.Arguments[0] == "#suomiscene" || e.Arguments[0] == "#pyfibot.test") {
					go rexpl(&config, conn, e, e.Message())
					return
				}

				// handle commands, command needs to be at least one character past prefix
				if strings.HasPrefix(e.Message(), ".") && len(e.Message()) > 1 {
					go handleCommand(&config, conn, e, e.Message()[1:])
					return
				}

				// If it wasn't a command, check if it's an URL
				for _, word := range words {
					if !strings.HasPrefix(word, "http") {
						continue
					}

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

			// Handle nonstandard ports
			var port = 6667
			if network.Port != 0 {
				port = network.Port
			}

			// Connect to the IRC server
			server := fmt.Sprintf("%s:%d", network.Server, port)
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
