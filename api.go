package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"path/filepath"

	"github.com/pressly/chi"
	"github.com/pressly/chi/middleware"
	"github.com/sirupsen/logrus"
)

var templ = template.Must(template.ParseGlob("./template/*.html"))

type api struct {
	irc *Irc
}

func newAPI(irc *Irc) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CloseNotify)
	r.Use(middleware.Timeout(20 * time.Second))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Powered-By", "Dank Memes")
			next.ServeHTTP(w, r)
		})
	})

	a := &api{
		irc: irc,
	}

	r.Get("/", index)
	r.Route("/:channel", func(r chi.Router) {
		r.Use(a.channelCtx)
		r.Get("/", getLogs)
	})
	return r
}

func index(w http.ResponseWriter, r *http.Request) {
	templ.ExecuteTemplate(w, "index.html", cfg.Channels)
}

var validChannel = regexp.MustCompile(`^\w{1,30}$`)

func (a *api) channelCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cName := chi.URLParam(r, "channel")
		cName = strings.ToLower(cName)
		if !validChannel.MatchString(cName) {
			http.Error(w, "Not Found", 404)
			return
		}
		a.irc.mu.RLock()
		defer a.irc.mu.RUnlock()
		l := a.irc.channels[cName]
		if l != nil {
			l.writer.Flush()
		}
		year, month, _ := time.Now().Date()
		dir := filepath.Join(cfg.LogPath, strconv.Itoa(year), month.String())
		file, err := os.Open(filepath.Join(dir, cName))
		if err != nil {
			logrus.Error(err)
			http.Error(w, "Not Found", 404)
			return
		}
		ctx := context.WithValue(r.Context(), "file", file)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getLogs(w http.ResponseWriter, r *http.Request) {
	file, ok := r.Context().Value("file").(*os.File)
	if !ok {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	defer file.Close()
	filter := r.URL.Query().Get("filter")
	logrus.Debug(filter)
	filter = strings.ToLower(filter)
	filters := strings.Split(filter, ",")
	limitStr := r.URL.Query().Get("limit")
	limit := 300
	if limitStr != "" {
		lm, err := strconv.Atoi(limitStr)
		if err != nil {
			logrus.Error(err)
		} else {
			limit = lm
		}
	}

	limitBytes := int64(1024 * limit)
	if stat, err := file.Stat(); err == nil {
		if stat.Size() > limitBytes {
			_, err := file.Seek(-limitBytes, 2)
			if err != nil {
				logrus.Error(err)
				http.Error(w, "Internal Server Error", 500)
				return
			}
		}
	}
	if limitBytes > 1024*1024 { // 1 MB
		limitBytes = 1024 * 1024
	}
	buf := bufio.NewReaderSize(file, int(limitBytes))
	lines := make([]*Message, 0, 300)
	i := 0
	start := time.Now()
	for {
		if i%50 == 0 && i > 300 {
			if time.Since(start) > time.Second*3 {
				w.WriteHeader(400)
				w.Write([]byte(`<!doctype html><html><body>
                nice Server
                <img src="https://cdn.betterttv.net/emote/550b225fff8ecee922d2a3b2/2x">
                </body></html>`))
				return
			}
		}
		i++
		line, err := buf.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			logrus.Error(err)
			break
		}
		msg := &Message{}
		err = json.Unmarshal(line, msg)
		if err != nil {
			logrus.Debug(len(lines), " ", i)
			if len(lines) > 1 {
				logrus.Warning(err)
			}
			continue
		}
		if filter != "" {
			text := strings.ToLower(msg.Text)
			matched := true
			for _, f := range filters {
				if !strings.Contains(text, f) {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
		}
		messageWithEmotes(msg)
		msg.Time = msg.Time.In(timeZone)
		msg.TimeStamp = msg.Time.Format("02-01 15:04:05 MST")
		lines = append(lines, msg)
	}
	if len(lines) < 1 {
		http.Error(w, "No Logs Found", 404)
		return
	}
	data := struct {
		Channel  string
		Messages []*Message
	}{
		Channel:  lines[0].Channel,
		Messages: lines,
	}
	templ.ExecuteTemplate(w, "chat.html", data)
}

func messageWithEmotes(msg *Message) {
	escaped := html.EscapeString(msg.Text)
	if len(msg.Emotes) < 1 {
		msg.TextWithEmotes = template.HTML(escaped)
		return
	}
	for _, emote := range msg.Emotes {
		var url string
		switch emote.Type {
		case "twitch":
			url = fmt.Sprintf("https://static-cdn.jtvnw.net/emoticons/v1/%s/1.0", emote.ID)

		}
		escaped = strings.Replace(escaped, emote.Name,
			`<img src="`+url+`" class="emote" alt="`+emote.Name+`">`,
			-1)
	}
	msg.TextWithEmotes = template.HTML(escaped)
}
