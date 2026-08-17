package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"time"
)

// DisclosureTerm is the maximum allowed age of the beacon before disclosure.
const DisclosureTerm = time.Hour * 24 * 21

// checkBeacon reports whether the beacon timestamp in buf is still within
// DisclosureTerm of now.
func checkBeacon(buf []byte, now time.Time) (bool, error) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(buf)))
	if err != nil {
		return false, err
	}
	return now.Sub(t) < DisclosureTerm, nil
}

type renderIndexOptions struct {
	CurrentDate  string
	BeaconColor  string
	BeaconStatus string
	BeaconDate   string
	SecretKey    string
}

// renderIndex loads the template file and substitutes all placeholders.
func renderIndex(template string, opts renderIndexOptions) ([]byte, error) {
	buf, err := os.ReadFile(template)
	if err != nil {
		return nil, err
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
	return buf, nil
}

func createIndexFileFromBeaconFile(now time.Time, input string, output string) error {
	log.Println("now:", now.Format(time.RFC3339))

	buf, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	buf = bytes.TrimSpace(buf)

	log.Println("beacon:", string(buf))

	active, err := checkBeacon(buf, now)
	if err != nil {
		return err
	}
	log.Println("active:", active)

	opts := renderIndexOptions{
		CurrentDate: now.Format(time.RFC3339),
		BeaconDate:  string(buf),
	}

	if active {
		opts.BeaconColor = "success"
		opts.BeaconStatus = "ACTIVE"
		opts.SecretKey = "N/A"
	} else {
		opts.BeaconColor = "danger"
		opts.BeaconStatus = "INACTIVE"
		opts.SecretKey = strings.TrimSpace(os.Getenv("SECRET_KEY"))
	}

	log.Printf("options: current_date=%s, beacon_color=%s, beacon_status=%s",
		opts.CurrentDate, opts.BeaconColor, opts.BeaconStatus)

	out, err := renderIndex("index.src.html", opts)
	if err != nil {
		return err
	}

	log.Println("rendered:", output)

	return os.WriteFile(output, out, 0644)
}

func main() {
	if err := createIndexFileFromBeaconFile(time.Now(), "beacon.txt", "index.html"); err != nil {
		log.Fatal("exited with error: ", err)
	}
}
