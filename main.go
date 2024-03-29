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
	DisclosureTerm = time.Hour * 24 * 21

	EnvKeySecret = "SECRET_KEY"

	FileTemplate = "index.gohtml"
	FileIndex    = "index.html"
	FileBeacon   = "beacon.txt"

	TimeLayoutBeacon = time.RFC3339
	TimeLayoutOutput = "2006-01-02 15:04:05 (-0700)"
)

func LoadBeacon() (beacon string, active bool, err error) {
	log.Println("loading beacon:", FileBeacon)

	var buf []byte
	if buf, err = os.ReadFile(FileBeacon); err != nil {
		return
	}

	log.Println("parsing beacon:", string(buf))

	var t time.Time
	if t, err = time.Parse(TimeLayoutBeacon, string(bytes.TrimSpace(buf))); err != nil {
		return
	}

	beacon, active = t.Format(TimeLayoutOutput), time.Since(t) < DisclosureTerm

	log.Println("beacon:", beacon)
	log.Println("beacon active:", active)

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
	Beacon string
	Active bool
	Secret string
	Now    string
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

	data.Now = time.Now().Format(TimeLayoutOutput)

	if data.Beacon, data.Active, err = LoadBeacon(); err != nil {
		return
	}

	if data.Active {
		data.Secret = "N/A"
	} else {
		data.Secret = strings.TrimSpace(os.Getenv(EnvKeySecret))
	}

	if err = RenderIndexHTML(data); err != nil {
		return
	}

}
