package main

import (
	"log/slog"

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

func (r TitleResponse) errMsg() string { return r.ErrorMessage }

// fetchLambdaTitle fetches the title using a Lambda function.
func fetchLambdaTitle(config *Config, payload *TitlePayload) (string, error) {
	response, err := fetchLambda[TitleResponse](config.LambdaTitle, payload)
	if err != nil {
		return "", err
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
