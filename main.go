package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"time"
)

const (
	DisclosureTerm = time.Hour * 24 * 21
)

func checkBeacon(buf []byte, now time.Time) (active bool, err error) {
	var t time.Time
	if t, err = time.Parse(time.RFC3339, strings.TrimSpace(string(buf))); err != nil {
		return
	}

	active = now.Sub(t) < DisclosureTerm
	return
}

type renderIndexOptions struct {
	CurrentDate  string
	BeaconColor  string
	BeaconStatus string
	BeaconDate   string
	SecretKey    string
}

func renderIndex(opts renderIndexOptions) (buf []byte, err error) {
	if buf, err = os.ReadFile("index.src.html"); err != nil {
		return
	}
	for k, v := range map[string]string{
		"___CURRENT_DATE___":  opts.CurrentDate,
		"___BEACON_COLOR___":  opts.BeaconColor,
		"___BEACON_STATUS___": opts.BeaconStatus,
		"___BEACON_DATE___":   opts.BeaconDate,
		"___SECRET_KEY___":    opts.SecretKey,
	} {
		buf = bytes.ReplaceAll(buf, []byte(k), []byte(v))
	}
	return
}

func createIndexFileFromBeaconFile(now time.Time, input string, output string) (err error) {
	log.Println("now:", now.Format(time.RFC3339))

	var buf []byte
	if buf, err = os.ReadFile(input); err != nil {
		return
	}
	buf = bytes.TrimSpace(buf)

	log.Println("beacon:", string(buf))

	var active bool
	if active, err = checkBeacon(buf, now); err != nil {
		return
	}
	log.Println("active:", active)

	var opts renderIndexOptions

	opts.CurrentDate = now.Format(time.RFC3339)
	opts.BeaconDate = string(buf)

	if active {
		opts.BeaconColor = "success"
		opts.BeaconStatus = "ACTIVE"
		opts.SecretKey = "N/A"
	} else {
		opts.BeaconColor = "danger"
		opts.BeaconStatus = "INACTIVE"
		opts.SecretKey = strings.TrimSpace(os.Getenv("SECRET_KEY"))
	}

	log.Printf("options: current_data=%s, beacon_color=%s, beacon_status=%s", opts.CurrentDate, opts.BeaconColor, opts.BeaconStatus)

	var out []byte
	if out, err = renderIndex(opts); err != nil {
		return
	}

	log.Println("rendered:", output)

	if err = os.WriteFile(output, out, 0644); err != nil {
		return
	}

	return
}

func main() {
	if err := createIndexFileFromBeaconFile(time.Now(), "beacon.txt", "index.html"); err != nil {
		log.Println("exited with error:", err.Error())
		os.Exit(1)
	}
}
