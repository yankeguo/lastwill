package main

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(file)
}

func productionTemplate(t *testing.T) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "index.src.html")
}

func TestParseBeacon(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		content string
		active  bool
		wantAt  time.Time
		wantErr error
	}{
		{
			name:    "inside term",
			content: now.Add(-time.Hour).Format(time.RFC3339) + "\n",
			active:  true,
			wantAt:  now.Add(-time.Hour),
		},
		{
			name:    "exactly at term",
			content: now.Add(-DisclosureTerm).Format(time.RFC3339),
			active:  false,
			wantAt:  now.Add(-DisclosureTerm),
		},
		{
			name:    "one second inside term",
			content: "  " + now.Add(-DisclosureTerm+time.Second).Format(time.RFC3339) + "\t",
			active:  true,
			wantAt:  now.Add(-DisclosureTerm + time.Second),
		},
		{
			name:    "one second past term",
			content: now.Add(-DisclosureTerm - time.Second).Format(time.RFC3339),
			active:  false,
			wantAt:  now.Add(-DisclosureTerm - time.Second),
		},
		{
			name:    "future within skew",
			content: now.Add(MaxFutureSkew).Format(time.RFC3339),
			active:  true,
			wantAt:  now.Add(MaxFutureSkew),
		},
		{
			name:    "offset timezone is the same instant",
			content: "2026-10-16T16:00:00+08:00",
			active:  true,
			wantAt:  now,
		},
		{
			name:    "bom and surrounding space",
			content: "\uFEFF\n" + now.Format(time.RFC3339) + "\n",
			active:  true,
			wantAt:  now,
		},
		{
			name:    "future beyond skew",
			content: now.Add(MaxFutureSkew + time.Second).Format(time.RFC3339),
			wantErr: errBeaconFuture,
		},
		{
			name:    "far future",
			content: now.Add(DisclosureTerm).Format(time.RFC3339),
			wantErr: errBeaconFuture,
		},
		{
			name:    "empty",
			content: "",
			wantErr: errBeaconEmpty,
		},
		{
			name:    "whitespace",
			content: " \n\t ",
			wantErr: errBeaconEmpty,
		},
		{
			name:    "invalid",
			content: "not-a-time",
			wantErr: errBeaconInvalid,
		},
		{
			name:    "trailing junk",
			content: now.Format(time.RFC3339) + " extra",
			wantErr: errBeaconInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at, active, err := parseBeacon([]byte(tt.content), now)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.True(t, at.IsZero())
				require.False(t, active)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.active, active)
			require.True(t, at.Equal(tt.wantAt), "got %s want %s", at, tt.wantAt)
		})
	}
}

func TestParseBeacon_ZeroNow(t *testing.T) {
	_, active, err := parseBeacon([]byte(time.Now().Format(time.RFC3339)), time.Time{})
	require.Error(t, err)
	require.False(t, active)
}

func TestNormalizeSecret(t *testing.T) {
	got, err := normalizeSecret("  real-key  ")
	require.NoError(t, err)
	require.Equal(t, "real-key", got)

	_, err = normalizeSecret(" \n\t ")
	require.ErrorIs(t, err, errSecretMissing)

	_, err = normalizeSecret("N/A")
	require.ErrorIs(t, err, errSecretRejected)

	_, err = normalizeSecret(string([]byte{0xff, 0xfe}))
	require.ErrorIs(t, err, errSecretRejected)

	_, err = normalizeSecret(strings.Repeat("k", maxSecretBytes+1))
	require.ErrorIs(t, err, errSecretRejected)

	got, err = normalizeSecret(strings.Repeat("k", maxSecretBytes))
	require.NoError(t, err)
	require.Len(t, got, maxSecretBytes)
}

func TestRenderIndex_ProductionTemplate(t *testing.T) {
	src, err := os.ReadFile(productionTemplate(t))
	require.NoError(t, err)
	require.NotContains(t, string(src), "21 days")
	require.NotContains(t, string(src), "___")
	require.Contains(t, string(src), "{{.DisclosureDays}}")
	require.Contains(t, string(src), "{{.SecretKey}}")

	now := time.Date(2021, 8, 10, 0, 0, 0, 0, time.UTC)
	beaconAt := now.Add(-time.Hour)
	secret := "line1\nline2 <script>alert(\"x\")</script> & co {{end}}"

	activeOpts, err := newRenderOptions(now, beaconAt, true, secret)
	require.NoError(t, err)
	require.Equal(t, activeSecretPlaceholder, activeOpts.SecretKey)

	activeHTML, err := renderIndex(src, activeOpts)
	require.NoError(t, err)
	activeText := string(activeHTML)
	require.Contains(t, activeText, ">"+now.UTC().Format(time.RFC3339)+"</em>")
	require.Contains(t, activeText, "status-success")
	require.Contains(t, activeText, "BEACON ACTIVE")
	require.Contains(t, activeText, ">"+beaconAt.UTC().Format(time.RFC3339)+"</span>")
	require.Contains(t, activeText, ">N/A</code>")
	require.Contains(t, activeText, fmt.Sprintf("%d days", disclosureDays()))
	require.NotContains(t, activeText, secret)
	require.NotContains(t, activeText, "{{")

	inactiveOpts, err := newRenderOptions(now, beaconAt, false, secret)
	require.NoError(t, err)
	inactiveHTML, err := renderIndex(src, inactiveOpts)
	require.NoError(t, err)
	inactiveText := string(inactiveHTML)
	require.Contains(t, inactiveText, "status-danger")
	require.Contains(t, inactiveText, "BEACON INACTIVE")
	require.Contains(t, inactiveText, html.EscapeString(secret))
	require.NotContains(t, inactiveText, "<script>alert")
	require.NotContains(t, inactiveText, ">N/A</code>")
	require.Contains(t, inactiveText, "line1\nline2")
}

func TestRenderIndex_RejectsInconsistentOptions(t *testing.T) {
	src := []byte(`BEACON {{.BeaconStatus}} {{.SecretKey}}`)
	_, err := renderIndex(src, renderIndexOptions{
		CurrentDate:    "2026-10-16T00:00:00Z",
		BeaconDate:     "2026-10-16T00:00:00Z",
		BeaconColor:    "danger",
		BeaconStatus:   "ACTIVE",
		SecretKey:      "real-key",
		DisclosureDays: 21,
	})
	require.Error(t, err)
}

func TestRenderIndex_MissingField(t *testing.T) {
	_, err := renderIndex([]byte(`{{.Missing}}`), renderIndexOptions{
		CurrentDate:    "2026-10-16T00:00:00Z",
		BeaconDate:     "2026-10-16T00:00:00Z",
		BeaconColor:    "success",
		BeaconStatus:   "ACTIVE",
		SecretKey:      "N/A",
		DisclosureDays: 21,
	})
	require.Error(t, err)
}

func TestRun_ActiveAndInactive(t *testing.T) {
	now := time.Date(2021, 8, 10, 0, 0, 0, 0, time.UTC)
	secret := `pk-test-<>&"'{{end}}`

	dir := t.TempDir()
	beacon := filepath.Join(dir, "beacon.txt")
	output := filepath.Join(dir, "dist", "index.html")
	require.NoError(t, os.WriteFile(beacon, []byte(now.Add(-DisclosureTerm/2).Format(time.RFC3339)+"\n"), 0o644))

	require.NoError(t, run(now, secret, beacon, productionTemplate(t), output))
	buf, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(buf), "BEACON ACTIVE")
	require.Contains(t, string(buf), ">N/A</code>")
	require.NotContains(t, string(buf), secret)
	info, err := os.Stat(output)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	require.NoError(t, os.WriteFile(beacon, []byte(now.Add(-DisclosureTerm*2).Format(time.RFC3339)+"\n"), 0o644))
	require.NoError(t, run(now, "  "+secret+"  ", beacon, productionTemplate(t), output))
	buf, err = os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(buf), "BEACON INACTIVE")
	require.Contains(t, string(buf), html.EscapeString(secret))
	require.NotContains(t, string(buf), ">N/A</code>")
}

func TestRun_RejectsBadSecretWithoutWriting(t *testing.T) {
	now := time.Date(2021, 8, 10, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	beacon := filepath.Join(dir, "beacon.txt")
	output := filepath.Join(dir, "index.html")
	require.NoError(t, os.WriteFile(beacon, []byte(now.Add(-DisclosureTerm*2).Format(time.RFC3339)), 0o644))
	require.NoError(t, os.WriteFile(output, []byte("previous"), 0o644))

	for _, secret := range []string{"", "   ", "N/A", string([]byte{0xff})} {
		err := run(now, secret, beacon, productionTemplate(t), output)
		require.Error(t, err)
		buf, readErr := os.ReadFile(output)
		require.NoError(t, readErr)
		require.Equal(t, "previous", string(buf))
	}
}

func TestRun_PreservesOutputWhenBeaconIsRejected(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	beacon := filepath.Join(dir, "beacon.txt")
	output := filepath.Join(dir, "index.html")
	require.NoError(t, os.WriteFile(output, []byte("previous"), 0o644))

	cases := []struct {
		name    string
		content string
		wantErr error
	}{
		{"invalid", "not-a-time", errBeaconInvalid},
		{"future", now.Add(48 * time.Hour).Format(time.RFC3339), errBeaconFuture},
		{"empty", "\n", errBeaconEmpty},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(beacon, []byte(tt.content), 0o644))
			err := run(now, "a-usable-secret", beacon, productionTemplate(t), output)
			require.ErrorIs(t, err, tt.wantErr)
			buf, readErr := os.ReadFile(output)
			require.NoError(t, readErr)
			require.Equal(t, "previous", string(buf))
		})
	}
}

func TestRun_BeaconTooLarge(t *testing.T) {
	dir := t.TempDir()
	beacon := filepath.Join(dir, "beacon.txt")
	require.NoError(t, os.WriteFile(beacon, bytes.Repeat([]byte("a"), maxBeaconBytes+1), 0o644))

	err := run(time.Now(), "a-usable-secret", beacon, productionTemplate(t), filepath.Join(dir, "index.html"))
	require.ErrorIs(t, err, errFileTooLarge)
	_, statErr := os.Stat(filepath.Join(dir, "index.html"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestRun_MissingBeacon(t *testing.T) {
	dir := t.TempDir()
	err := run(time.Now(), "a-usable-secret", filepath.Join(dir, "missing.txt"), productionTemplate(t), filepath.Join(dir, "index.html"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRun_RefusesToPublishKeyOnActivePage(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	secret := "unique-disclosure-secret"
	dir := t.TempDir()
	beacon := filepath.Join(dir, "beacon.txt")
	output := filepath.Join(dir, "index.html")
	templatePath := filepath.Join(dir, "index.src.html")
	require.NoError(t, os.WriteFile(beacon, []byte(now.Format(time.RFC3339)), 0o644))
	require.NoError(t, os.WriteFile(templatePath, []byte(secret+" BEACON {{.BeaconStatus}} <code>{{.SecretKey}}</code>"), 0o644))
	require.NoError(t, os.WriteFile(output, []byte("previous"), 0o644))

	err := run(now, secret, beacon, templatePath, output)
	require.ErrorIs(t, err, errKeyLeaked)
	buf, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	require.Equal(t, "previous", string(buf))
}

func TestRun_RefusesEscapedKeyOnActivePage(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	secret := "unique<script>key"
	dir := t.TempDir()
	beacon := filepath.Join(dir, "beacon.txt")
	output := filepath.Join(dir, "index.html")
	templatePath := filepath.Join(dir, "index.src.html")
	require.NoError(t, os.WriteFile(beacon, []byte(now.Format(time.RFC3339)), 0o644))
	page := html.EscapeString(secret) + ` BEACON {{.BeaconStatus}} <code>{{.SecretKey}}</code>`
	require.NoError(t, os.WriteFile(templatePath, []byte(page), 0o644))
	require.NoError(t, os.WriteFile(output, []byte("previous"), 0o644))

	err := run(now, secret, beacon, templatePath, output)
	require.ErrorIs(t, err, errKeyLeaked)
	buf, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	require.Equal(t, "previous", string(buf))
}

func TestRun_RefusesInactivePageWithoutKey(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	beacon := filepath.Join(dir, "beacon.txt")
	output := filepath.Join(dir, "index.html")
	templatePath := filepath.Join(dir, "index.src.html")
	require.NoError(t, os.WriteFile(beacon, []byte(now.Add(-DisclosureTerm*2).Format(time.RFC3339)), 0o644))
	require.NoError(t, os.WriteFile(templatePath, []byte(`BEACON {{.BeaconStatus}}`), 0o644))

	err := run(now, "a-usable-secret", beacon, templatePath, output)
	require.ErrorIs(t, err, errKeyMissing)
	_, statErr := os.Stat(output)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "index.html")

	require.Error(t, writeFileAtomic(path, nil, 0o644))
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, writeFileAtomic(path, []byte("v1"), 0o644))
	require.NoError(t, writeFileAtomic(path, []byte("v2"), 0o644))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "v2", string(got))

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "index.html", entries[0].Name())
}

func TestWorkflowPublishesBeaconAfterRender(t *testing.T) {
	buf, err := os.ReadFile(filepath.Join(moduleRoot(t), ".github", "workflows", "github-pages.yml"))
	require.NoError(t, err)
	text := string(buf)

	render := strings.Index(text, "go run .")
	publish := strings.Index(text, "git push")
	require.Greater(t, render, 0)
	require.Greater(t, publish, render)
	require.Contains(t, text, "SECRET_KEY:")
	require.Contains(t, text, "cancel-in-progress: true")
	require.Contains(t, text, "git add beacon.txt")
}

func FuzzParseBeacon(f *testing.F) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	f.Add(now.Format(time.RFC3339))
	f.Add("not-a-time")
	f.Add("")
	f.Add("\uFEFF" + now.Add(48*time.Hour).Format(time.RFC3339))
	f.Fuzz(func(t *testing.T, content string) {
		at, active, err := parseBeacon([]byte(content), now)
		if err != nil {
			require.False(t, active)
			require.True(t, at.IsZero())
			return
		}
		require.False(t, at.After(now.Add(MaxFutureSkew)))
		require.Equal(t, now.Sub(at) < DisclosureTerm, active)
	})
}

func FuzzNormalizeSecret(f *testing.F) {
	f.Add("a-reasonable-key")
	f.Add("")
	f.Add("N/A")
	f.Add("  spaced  ")
	f.Fuzz(func(t *testing.T, secret string) {
		got, err := normalizeSecret(secret)
		if err != nil {
			require.Empty(t, got)
			return
		}
		require.Equal(t, strings.TrimSpace(got), got)
		require.NotEmpty(t, got)
		require.NotEqual(t, activeSecretPlaceholder, got)
		require.LessOrEqual(t, len(got), maxSecretBytes)
	})
}
