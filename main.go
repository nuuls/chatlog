package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

type config struct {
	Host      string   `json:"host"`
	LogPath   string   `json:"logPath"`
	Channels  []string `json:"channels"`
	IrcServer string   `json:"ircServer"`
}

var cfg *config
var timeZone *time.Location

func main() {
	timeZone, _ = time.LoadLocation("CET")
	loadConfig()
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: "02-01 15:04:05.000",
	})
	logrus.SetLevel(logrus.DebugLevel)
	i := dial("")
	for _, channel := range cfg.Channels {
		i.join(channel)
	}
	srv := &http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      newAPI(i),
		Addr:         cfg.Host,
	}
	log.Fatal(srv.ListenAndServe())
}

func loadConfig() {
	bs, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}
	cfg = &config{}
	err = json.Unmarshal(bs, cfg)
	if err != nil {
		panic(err)
	}
}
