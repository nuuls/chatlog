package main

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"time"

	"path/filepath"

	"github.com/sirupsen/logrus"
)

type logger struct {
	file    *os.File
	writer  *bufio.Writer
	channel string
	write   chan []byte
}

func newLogger(channel string) *logger {
	year, month, _ := time.Now().Date()
	dir := filepath.Join(cfg.LogPath, strconv.Itoa(year), month.String())
	err := os.MkdirAll(dir, os.ModeDir)
	if err != nil {
		logrus.Fatal(err)
	}
	p := filepath.Join(dir, channel)
	file, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, os.ModeAppend)
	if err != nil {
		logrus.Debug(err)
		file, err = os.Create(p)
		if err != nil {
			logrus.Fatal(err)
		}
		file.Close()
		file, err = os.OpenFile(p, os.O_WRONLY, os.ModeAppend)
		if err != nil {
			log.Fatal(err)
		}
	}
	l := &logger{
		file:    file,
		writer:  bufio.NewWriterSize(file, 1024*32),
		channel: channel,
		write:   make(chan []byte, 5),
	}
	go l.run()
	return l
}

func (l *logger) run() {
	defer l.writer.Flush()
	defer l.file.Close()
	for line := range l.write {
		_, err := l.writer.Write(append(line, '\n'))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"channel": l.channel,
				"msg":     string(line),
			}).WithError(err).Fatal("cannot write to file")
		}
		if l.writer.Buffered() > 1024*32 {
			err = l.writer.Flush()
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"channel": l.channel,
					"msg":     string(line),
				}).WithError(err).Fatal("cannot write to file")
			}
		}
	}
}
