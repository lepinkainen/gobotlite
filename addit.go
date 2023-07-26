package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	irc "github.com/thoj/go-ircevent"
)

type Quote struct {
	TimeAdded string `json:"time_added"`
	Topic     string `json:"topic"`
	Handle    string `json:"handle"`
	Content   string `json:"content"`
}

func rexpl(config *Config, conn *irc.Connection, e *irc.Event, commandStr string) {
	client := &http.Client{}
	var req *http.Request
	var err error

	words := strings.Fields(e.Message())
	topic := strings.Join(words[1:], " ")

	var url string

	if topic != "" {
		// random with specific search
		url = fmt.Sprintf("%s/rexpl/?q=%s", config.Addit.Endpoint, topic)
	} else {
		// just random
		url = fmt.Sprintf("%s/rexpl/", config.Addit.Endpoint)
	}

	//fmt.Println("Querying URL", url)

	req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	req.Header.Set("Authorization", fmt.Sprintf("Token %s", config.Addit.APIKey))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}

	if err != nil || resp.StatusCode != 200 {
		fmt.Println(err)
		return
	}

	quote := &Quote{}
	if err := json.NewDecoder(resp.Body).Decode(quote); err != nil {
		fmt.Println(err)
		return
	}

	conn.Privmsg(e.Arguments[0], fmt.Sprintf("'%s': %s", quote.Topic, quote.Content))
}
