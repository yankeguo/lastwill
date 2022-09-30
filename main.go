package main

import (
	"bytes"
	"html/template"
	"log"
	"os"
	"strings"
	"time"
)

const (
	DisclosureTerm = time.Hour * 24 * 1

	EnvKeySecret = "SECRET_KEY"

	FileTemplate = "index.gohtml"
	FileIndex    = "index.html"
	FileBeacon   = "beacon.txt"
)

func LoadBeacon() (beacon string, alive bool, err error) {
	log.Println("loading beacon:", FileBeacon)

	var buf []byte
	if buf, err = os.ReadFile(FileBeacon); err != nil {
		return
	}

	log.Println("parsing beacon:", string(buf))

	var t time.Time
	if t, err = time.Parse(time.RFC3339, string(bytes.TrimSpace(buf))); err != nil {
		return
	}

	beacon, alive = t.Format("2006-01-02 15:04:05 (-0700)"), time.Now().Sub(t) < DisclosureTerm

	log.Println("beacon:", beacon)
	log.Println("beacon alive:", alive)

	return
}

func RenderIndexHTML(data Data) (err error) {
	log.Println("loading template:", FileTemplate)

	var buf []byte
	if buf, err = os.ReadFile(FileTemplate); err != nil {
		return
	}

	log.Println("parsing template")

	var tpl *template.Template
	if tpl, err = template.New("__index__").Parse(string(buf)); err != nil {
		return
	}

	log.Println("rendering:", FileIndex)

	out := &bytes.Buffer{}

	if err = tpl.Execute(out, data); err != nil {
		return
	}

	log.Println("writing:", FileIndex)

	if err = os.WriteFile(FileIndex, out.Bytes(), 0644); err != nil {
		return
	}

	return
}

type Data struct {
	Beacon    string
	Alive     bool
	SecretKey string
}

func main() {
	var err error
	defer func() {
		if err == nil {
			return
		}
		log.Println("exited with error:", err.Error())
		os.Exit(1)
	}()

	var data Data

	if data.Beacon, data.Alive, err = LoadBeacon(); err != nil {
		return
	}

	if data.Alive {
		data.SecretKey = "N/A"
	} else {
		data.SecretKey = strings.TrimSpace(os.Getenv(EnvKeySecret))
	}

	if err = RenderIndexHTML(data); err != nil {
		return
	}

}
