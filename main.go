package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

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
	Security      struct {
		AllowInsecureTLS bool `yaml:"allowInsecureTLS"`
		RateLimit        struct {
			Enabled bool `yaml:"enabled"`
			Rate    int  `yaml:"rate"`
			Burst   int  `yaml:"burst"`
		} `yaml:"rateLimit"`
	} `yaml:"security"`
	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"logging"`
}

var Version = "development"

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

func connectWithRetry(conn *irc.Connection, server string) error {
	backoff := time.Second
	maxBackoff := time.Minute * 5

	for {
		err := conn.Connect(server)
		if err == nil {
			return nil
		}

		log.Printf("Connection failed: %v, retrying in %v", err, backoff)
		time.Sleep(backoff)

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func main() {
	config := Config{}

	log.Printf("Starting bot version %s", Version)

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

			conn.AddCallback("433", func(e *irc.Event) {
				log.Printf("Nickname is already in use")
				conn.Nick(config.Nickname + "_")
			})

			// Handle CTCP VERSION
			conn.AddCallback("CTCP_VERSION", func(e *irc.Event) {
				conn.SendRawf("NOTICE %s :\x01VERSION gobotlite - https://github.com/lepinkainen/gobotlite\x01", e.Nick)
			})

			// Handle CTCP TIME
			conn.AddCallback("CTCP_TIME", func(e *irc.Event) {
				conn.SendRawf("NOTICE %s :\x01TIME %s\x01", e.Nick, time.Now().Format(time.RFC1123))
			})

			// Handle CTCP PING
			conn.AddCallback("CTCP_PING", func(e *irc.Event) {
				conn.SendRawf("NOTICE %s :\x01PING %s\x01", e.Nick, e.Arguments[1])
			})

			// Handle kicks
			conn.AddCallback("KICK", func(e *irc.Event) {
				if e.Arguments[1] == config.Nickname {
					log.Printf("Kicked from %s by %s, rejoining...", e.Arguments[0], e.Nick)
					conn.Join(e.Arguments[0])
				}
			})

			// Handle invites
			conn.AddCallback("INVITE", func(e *irc.Event) {
				log.Printf("Invited to %s by %s", e.Arguments[1], e.Nick)
				//conn.Join(e.Arguments[1])
			})

			// Add callback for PRIVMSG
			conn.AddCallback("PRIVMSG", func(e *irc.Event) {
				var channel = e.Arguments[0]
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

				// handle commands, command needs to be at least one character past prefix
				if strings.HasPrefix(e.Message(), ".") && len(e.Message()) > 1 {
					//nolint:errcheck
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
						// ignore if prefixed with *
						// matrix bridges do this when linking to Discord and it's annoying AF
						if strings.HasPrefix(e.Message(), "*") {
							log.Printf("Ignoring URL: %s", u.String())

						} else {
							// Valid URL detected, handle accordingly
							log.Printf("URL detected on %s: %s", channel, u.String())
							go handleURL(&config, conn, e, u.String())
						}
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
			err = connectWithRetry(conn, server)
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
