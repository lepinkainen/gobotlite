package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	irc "github.com/fluffle/goirc/client"
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

func connectWithRetry(conn *irc.Conn, server string) error {
	backoff := time.Second
	maxBackoff := time.Minute * 5

	for {
		err := conn.ConnectTo(server)
		if err == nil {
			return nil
		}

		slog.Warn("Connection failed, retrying", "error", err, "backoff", backoff)
		time.Sleep(backoff)

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func main() {
	config := Config{}

	slog.Info("Starting bot", "version", Version)

	// Configure Viper
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// Read configuration
	err := viper.ReadInConfig()
	if err != nil {
		slog.Error("Error reading config file", "error", err)
		os.Exit(1)
	}

	// Unmarshal the configuration into the Config struct
	err = viper.Unmarshal(&config)
	if err != nil {
		slog.Error("Error parsing config file", "error", err)
		os.Exit(1)
	}

	err = config.Validate()
	if err != nil {
		slog.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}

	var wg sync.WaitGroup

	for _, network := range config.Networks {
		wg.Add(1)

		go func(network Network) {
			defer wg.Done()

			// Create new IRC connection with nickname from config
			cfg := irc.NewConfig(config.Nickname)
			cfg.SSL = network.UseTLS
			cfg.SSLConfig = &tls.Config{InsecureSkipVerify: true}
			conn := irc.Client(cfg)

			// Add callback for IRC connection
			conn.HandleFunc(irc.CONNECTED, func(conn *irc.Conn, line *irc.Line) {
				for _, channel := range network.Channels {
					// Default to #channels
					if !strings.HasPrefix(channel, "#") {
						channel = "#" + channel
					}
					conn.Join(channel)
				}
			})

			conn.HandleFunc("366", func(conn *irc.Conn, line *irc.Line) {
				slog.Info("Joined channel", "channel", line.Args[1])
			})

			conn.HandleFunc("433", func(conn *irc.Conn, line *irc.Line) {
				slog.Warn("Nickname is already in use")
				conn.Nick(config.Nickname + "_")
			})

			// Handle CTCP VERSION
			conn.HandleFunc("CTCP_VERSION", func(conn *irc.Conn, line *irc.Line) {
				conn.Notice(line.Nick, "\x01VERSION gobotlite - https://github.com/lepinkainen/gobotlite\x01")
			})

			// Handle CTCP TIME
			conn.HandleFunc("CTCP_TIME", func(conn *irc.Conn, line *irc.Line) {
				conn.Notice(line.Nick, "\x01TIME "+time.Now().Format(time.RFC1123)+"\x01")
			})

			// Handle CTCP PING
			conn.HandleFunc("CTCP_PING", func(conn *irc.Conn, line *irc.Line) {
				conn.Notice(line.Nick, "\x01PING "+line.Args[1]+"\x01")
			})

			// Handle kicks
			conn.HandleFunc("KICK", func(conn *irc.Conn, line *irc.Line) {
				if line.Args[1] == config.Nickname {
					slog.Info("Kicked from channel, rejoining", "channel", line.Args[0], "kicked_by", line.Nick)
					conn.Join(line.Args[0])
				}
			})

			// Handle invites
			conn.HandleFunc("INVITE", func(conn *irc.Conn, line *irc.Line) {
				slog.Info("Invited to channel", "channel", line.Args[1], "invited_by", line.Nick)
				//conn.Join(line.Args[1])
			})

			// Add callback for PRIVMSG
			conn.HandleFunc("PRIVMSG", func(conn *irc.Conn, line *irc.Line) {
				var channel = line.Args[0]
				// Ignore other bots
				if line.Nick == "Sinkko" {
					return
				}

				// slog.Debug("PRIVMSG received", "message", line.Text())

				words := strings.Fields(line.Text())

				// nothing to process
				if len(words) == 0 {
					return
				}

				// handle commands, command needs to be at least one character past prefix
				if strings.HasPrefix(line.Text(), ".") && len(line.Text()) > 1 {
					//nolint:errcheck
					go handleCommand(&config, conn, line, line.Text()[1:])
					return
				}

				// If it wasn't a command, check if it's an URL
				for _, word := range words {
					if !strings.HasPrefix(word, "http") {
						continue
					}

					u, err := url.Parse(word)

					if err != nil {
						slog.Debug("Error parsing potential URL", "url", word, "error", err)
					} else if u.Scheme != "" && u.Host != "" {
						// ignore if prefixed with *
						// matrix bridges do this when linking to Discord and it's annoying AF
						if strings.HasPrefix(line.Text(), "*") {
							slog.Debug("Ignoring URL", "url", u.String())

						} else {
							// Valid URL detected, handle accordingly
							slog.Info("URL detected", "channel", channel, "url", u.String())
							go handleURL(&config, conn, line, u.String())
						}
					}
				}
			})

			// Add callback for PING messages
			conn.HandleFunc("PING", func(conn *irc.Conn, line *irc.Line) { conn.Raw("PONG :" + line.Text()) })

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

			// Start IRC connection - fluffle/goirc doesn't have Loop(), connection is handled automatically
		}(network)
	}

	wg.Wait()
}
