package main

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DisclosureTerm is the maximum age of a beacon before the decryption
	// key is published. The comparison is strict: an age of exactly
	// DisclosureTerm is already expired. The term is 21 periods of 24 hours,
	// not calendar days.
	DisclosureTerm = 21 * 24 * time.Hour

	// MaxFutureSkew is how far ahead of now a beacon may be and still count
	// as a renewal. A small skew stays active so clock error cannot publish
	// the key. Anything further is rejected: a far-future timestamp must not
	// be able to suppress disclosure forever, and a failed run leaves the
	// previous page in place instead of announcing a healthy beacon.
	MaxFutureSkew = time.Hour

	// activeSecretPlaceholder is what the page shows while the key is withheld.
	// A real key equal to this string is rejected so the two states cannot be confused.
	activeSecretPlaceholder = "N/A"

	maxBeaconBytes   = 64
	maxTemplateBytes = 1 << 20
	maxSecretBytes   = 64 << 10
)

var (
	errBeaconEmpty    = errors.New("beacon is empty")
	errBeaconInvalid  = errors.New("beacon is not RFC3339")
	errBeaconFuture   = errors.New("beacon is too far in the future")
	errFileTooLarge   = errors.New("file exceeds size limit")
	errSecretMissing  = errors.New("SECRET_KEY is empty")
	errSecretRejected = errors.New("SECRET_KEY is not usable")
	errKeyLeaked      = errors.New("refusing to publish an active page that contains the decryption key")
	errKeyMissing     = errors.New("rendered index is missing the decryption key")
)

// parseBeacon reports the beacon instant and whether it is still inside DisclosureTerm.
// On any error the beacon is not active and the returned time is zero.
func parseBeacon(content []byte, now time.Time) (time.Time, bool, error) {
	if now.IsZero() {
		return time.Time{}, false, errors.New("current time is zero")
	}

	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	content = bytes.TrimSpace(content)
	if len(content) == 0 {
		return time.Time{}, false, errBeaconEmpty
	}

	at, err := time.Parse(time.RFC3339, string(content))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("%w: %q", errBeaconInvalid, string(content))
	}
	if at.After(now.Add(MaxFutureSkew)) {
		return time.Time{}, false, fmt.Errorf("%w: %s", errBeaconFuture, at.UTC().Format(time.RFC3339))
	}

	active := now.Sub(at) < DisclosureTerm
	return at, active, nil
}

// normalizeSecret checks the key that would be published if the beacon expired.
// It runs on every successful build, including while the beacon is active, so a
// missing key fails in CI while the operator is still alive.
func normalizeSecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", errSecretMissing
	}
	if len(secret) > maxSecretBytes {
		return "", fmt.Errorf("%w: longer than %d bytes", errSecretRejected, maxSecretBytes)
	}
	if !utf8.ValidString(secret) {
		return "", fmt.Errorf("%w: not valid UTF-8", errSecretRejected)
	}
	if secret == activeSecretPlaceholder {
		return "", fmt.Errorf("%w: must not be %s", errSecretRejected, activeSecretPlaceholder)
	}
	return secret, nil
}

type renderIndexOptions struct {
	CurrentDate    string
	BeaconColor    string
	BeaconStatus   string
	BeaconDate     string
	SecretKey      string
	DisclosureDays int
}

func (o renderIndexOptions) validate() error {
	switch o.BeaconColor {
	case "success", "danger":
	default:
		return fmt.Errorf("invalid beacon color %q", o.BeaconColor)
	}
	switch o.BeaconStatus {
	case "ACTIVE", "INACTIVE":
	default:
		return fmt.Errorf("invalid beacon status %q", o.BeaconStatus)
	}
	if o.CurrentDate == "" || o.BeaconDate == "" || o.SecretKey == "" || o.DisclosureDays <= 0 {
		return errors.New("render options are incomplete")
	}
	if o.BeaconStatus == "ACTIVE" {
		if o.BeaconColor != "success" || o.SecretKey != activeSecretPlaceholder {
			return errors.New("active beacon must withhold the decryption key")
		}
	}
	if o.BeaconStatus == "INACTIVE" {
		if o.BeaconColor != "danger" || o.SecretKey == activeSecretPlaceholder {
			return errors.New("inactive beacon must carry the decryption key")
		}
	}
	return nil
}

func disclosureDays() int {
	return int(DisclosureTerm / (24 * time.Hour))
}

func newRenderOptions(now, beaconAt time.Time, active bool, secret string) (renderIndexOptions, error) {
	opts := renderIndexOptions{
		CurrentDate:    now.UTC().Format(time.RFC3339),
		BeaconDate:     beaconAt.UTC().Format(time.RFC3339),
		DisclosureDays: disclosureDays(),
	}
	if active {
		// The real key is intentionally not copied onto the options while the
		// beacon is active. The renderer cannot publish a value it never receives.
		opts.BeaconColor = "success"
		opts.BeaconStatus = "ACTIVE"
		opts.SecretKey = activeSecretPlaceholder
		return opts, nil
	}

	key, err := normalizeSecret(secret)
	if err != nil {
		return renderIndexOptions{}, err
	}
	opts.BeaconColor = "danger"
	opts.BeaconStatus = "INACTIVE"
	opts.SecretKey = key
	return opts, nil
}

// renderIndex executes templateSrc. html/template escapes data in one pass, so a
// key cannot be re-scanned as markup or as another placeholder.
func renderIndex(templateSrc []byte, opts renderIndexOptions) ([]byte, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	tmpl, err := template.New("index").Option("missingkey=error").Parse(string(templateSrc))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, opts); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	if buf.Len() == 0 {
		return nil, errors.New("rendered index is empty")
	}
	return buf.Bytes(), nil
}

func validateRendered(out []byte, active bool, secret string) error {
	if len(out) == 0 {
		return errors.New("rendered index is empty")
	}

	status := "INACTIVE"
	if active {
		status = "ACTIVE"
	}
	// "BEACON ACTIVE" is not a substring of "BEACON INACTIVE".
	marker := "BEACON " + status
	if !bytes.Contains(out, []byte(marker)) {
		return fmt.Errorf("rendered index is missing %q", marker)
	}

	if active {
		if !bytes.Contains(out, []byte(activeSecretPlaceholder)) {
			return errors.New("rendered index is missing the withheld-key placeholder")
		}
		// Short keys can occur inside ordinary page text. A full key of this
		// length appearing contiguously means it was copied into the page.
		// Check both the raw key and its HTML escaping: a leaked key is stored
		// escaped, and that escaped text is what the browser shows.
		if secretVisible(out, secret) {
			return errKeyLeaked
		}
		return nil
	}

	if !bytes.Contains(out, []byte(html.EscapeString(secret))) {
		return errKeyMissing
	}
	return nil
}

// secretVisible reports whether secret, or the HTML text encoding of secret,
// appears in out. Keys shorter than 8 bytes are ignored: they collide with
// ordinary words and timestamps on the page.
func secretVisible(out []byte, secret string) bool {
	if len(secret) < 8 {
		return false
	}
	if bytes.Contains(out, []byte(secret)) {
		return true
	}
	escaped := html.EscapeString(secret)
	return escaped != secret && bytes.Contains(out, []byte(escaped))
}

func readLimited(path string, max int64) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("path is empty")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	buf, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if int64(len(buf)) > max {
		return nil, fmt.Errorf("%w: %s (limit %d bytes)", errFileTooLarge, path, max)
	}
	return buf, nil
}

// writeFileAtomic replaces path only after the full payload is durable in the
// same directory. A failed render leaves any previous page untouched.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if len(data) == 0 {
		return errors.New("refuse to write an empty file")
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("output path is empty")
	}

	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}

	// Best-effort: the replacement already succeeded, so a directory sync
	// failure must not fail the run or remove the published file.
	if dirf, err := os.Open(dir); err == nil {
		_ = dirf.Sync()
		_ = dirf.Close()
	}
	return nil
}

func run(now time.Time, secret, beaconPath, templatePath, outputPath string) error {
	if now.IsZero() {
		return errors.New("current time is zero")
	}
	if strings.TrimSpace(beaconPath) == "" || strings.TrimSpace(templatePath) == "" || strings.TrimSpace(outputPath) == "" {
		return errors.New("beacon, template, and output paths are required")
	}

	// Fail before touching the beacon or the published page. An empty key must
	// not look like a successful disclosure, and it must not pass silently for
	// years and only surface after the beacon expires.
	secret, err := normalizeSecret(secret)
	if err != nil {
		return err
	}

	raw, err := readLimited(beaconPath, maxBeaconBytes)
	if err != nil {
		return err
	}
	at, active, err := parseBeacon(raw, now)
	if err != nil {
		return err
	}

	opts, err := newRenderOptions(now, at, active, secret)
	if err != nil {
		return err
	}
	templateSrc, err := readLimited(templatePath, maxTemplateBytes)
	if err != nil {
		return err
	}
	out, err := renderIndex(templateSrc, opts)
	if err != nil {
		return err
	}
	if err := validateRendered(out, active, secret); err != nil {
		return err
	}
	if err := writeFileAtomic(outputPath, out, 0o644); err != nil {
		return err
	}

	if active {
		log.Printf("beacon=%s active=true age=%s; decryption key withheld", opts.BeaconDate, now.Sub(at).Truncate(time.Second))
		return nil
	}
	log.Printf("beacon=%s active=false age=%s; decryption key disclosed (%d bytes)", opts.BeaconDate, now.Sub(at).Truncate(time.Second), len(secret))
	return nil
}

func main() {
	if err := run(time.Now(), os.Getenv("SECRET_KEY"), "beacon.txt", "index.src.html", filepath.Join("dist", "index.html")); err != nil {
		log.Fatalf("lastwill: %s", err)
	}
}
