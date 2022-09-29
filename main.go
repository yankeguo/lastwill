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
	KeySecretKey = "SECRET_KEY"

	DisclosureTerm = time.Hour * 24 * 14

	FileTemplate = "index.gohtml"
	FileIndex    = "index.html"
	FileBeacon   = "beacon.txt"
)

func checkBeacon() (beacon string, alive bool, err error) {
	log.Println("checking beacon:", FileBeacon)

	var buf []byte
	if buf, err = os.ReadFile(FileBeacon); err != nil {
		return
	}

	buf = bytes.TrimSpace(buf)

	beacon = string(buf)

	log.Println("beacon:", beacon)

	var t time.Time
	if t, err = time.Parse(time.RFC3339, string(buf)); err != nil {
		return
	}

	alive = time.Now().Sub(t) < DisclosureTerm

	log.Println("beacon alive:", alive)

	return
}

func renderIndex(data Data) (err error) {
	log.Println("loading", FileTemplate)

	var buf []byte
	if buf, err = os.ReadFile(FileTemplate); err != nil {
		return
	}

	var tpl *template.Template
	if tpl, err = template.New("__index__").Parse(string(buf)); err != nil {
		return
	}

	log.Println("rendering", FileIndex)

	out := &bytes.Buffer{}

	if err = tpl.Execute(out, data); err != nil {
		return
	}

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

	if data.Beacon, data.Alive, err = checkBeacon(); err != nil {
		return
	}

	if data.Alive {
		data.SecretKey = "N/A"
	} else {
		data.SecretKey = strings.TrimSpace(os.Getenv(KeySecretKey))
	}

	if err = renderIndex(data); err != nil {
		return
	}

}
