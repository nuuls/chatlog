package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Irc struct {
	mu       sync.RWMutex
	channels map[string]*logger
	conn     net.Conn
}

func dial(addr string) *Irc {
	conn, err := tls.Dial("tcp", cfg.IrcServer, nil)
	if err != nil {
		logrus.Error(err)
		time.Sleep(time.Second * 5)
		return dial(addr)
	}
	i := &Irc{
		channels: map[string]*logger{},
		conn:     conn,
	}
	i.login()
	go i.read()
	return i
}

func (i *Irc) reconnect() {
	conn, err := tls.Dial("tcp", cfg.IrcServer, nil)
	if err != nil {
		logrus.Error(err)
		time.Sleep(time.Second * 5)
		i.reconnect()
		return
	}
	i.conn = conn
	i.login()
	go i.read()
	for _, channel := range cfg.Channels {
		i.join(channel)
	}
}

func (i *Irc) login() {
	i.send("NICK justinfan123")
	i.send("CAP REQ twitch.tv/tags")
	i.send("CAP REQ twitch.tv/commands")
}

func (i *Irc) join(channel string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	l := i.channels[channel]
	if l == nil {
		l = newLogger(channel)
		i.channels[channel] = l
	}
	i.send("JOIN #" + channel)
	logrus.Info("joined ", channel)
}

func (i *Irc) read() {
	reader := textproto.NewReader(bufio.NewReader(i.conn))
	for {
		line, err := reader.ReadLine()
		if err != nil {
			logrus.Error(err)
			i.reconnect()
			return
		}
		if strings.HasPrefix(line, "PING") {
			i.send(strings.Replace(line, "PING", "PONG", 1))
			continue
		}
		go i.handleMessage(line)
	}
}

func (i *Irc) handleMessage(line string) {
	msg := parseMessage(line)
	bs, err := json.Marshal(msg)
	if err != nil {
		logrus.Error(err)
		return
	}
	i.mu.RLock()
	l := i.channels[msg.Channel]
	i.mu.RUnlock()
	if l != nil {
		l.write <- bs
	}
}

func (i *Irc) send(msg string) error {
	logrus.Info(msg)
	_, err := i.conn.Write([]byte(msg + "\r\n"))
	if err != nil {
		logrus.Error(err)
	}
	return err
}
