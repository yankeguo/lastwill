package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckBeacon(t *testing.T) {
	now := time.Now()

	bct := []byte(now.Add(-DisclosureTerm*2).Format(time.RFC3339) + "    ")
	active, err := checkBeacon(bct, now)
	require.NoError(t, err)
	require.False(t, active)

	bct = []byte(now.Add(-DisclosureTerm/2).Format(time.RFC3339) + "    ")
	active, err = checkBeacon(bct, now)
	require.NoError(t, err)
	require.True(t, active)
}

func TestCheckBeacon_Invalid(t *testing.T) {
	now := time.Now()

	// invalid format
	_, err := checkBeacon([]byte("invalid"), now)
	require.Error(t, err)

	// empty content
	_, err = checkBeacon([]byte(""), now)
	require.Error(t, err)

	// whitespace only
	_, err = checkBeacon([]byte("   \n\t  "), now)
	require.Error(t, err)
}

func TestCheckBeacon_Boundary(t *testing.T) {
	now := time.Now()

	// exactly 21 days ago -> inactive (condition is <, not <=)
	bct := []byte(now.Add(-DisclosureTerm).Format(time.RFC3339))
	active, err := checkBeacon(bct, now)
	require.NoError(t, err)
	require.False(t, active)

	// 21 days + 1 second ago -> inactive
	bct = []byte(now.Add(-DisclosureTerm - time.Second).Format(time.RFC3339))
	active, err = checkBeacon(bct, now)
	require.NoError(t, err)
	require.False(t, active)

	// 20 days 23 hours 59 minutes 59 seconds ago -> active
	bct = []byte(now.Add(-DisclosureTerm + time.Second).Format(time.RFC3339))
	active, err = checkBeacon(bct, now)
	require.NoError(t, err)
	require.True(t, active)

	// future time -> active (negative duration)
	bct = []byte(now.Add(DisclosureTerm).Format(time.RFC3339))
	active, err = checkBeacon(bct, now)
	require.NoError(t, err)
	require.True(t, active)
}

func TestRenderIndex(t *testing.T) {
	buf, err := renderIndex(renderIndexOptions{
		CurrentDate:  "114",
		BeaconColor:  "514",
		BeaconStatus: "1919",
		BeaconDate:   "810",
		SecretKey:    "801",
	})
	require.NoError(t, err)
	require.Contains(t, string(buf), "<em>114</em>")
	require.Contains(t, string(buf), "status-514")
	require.Contains(t, string(buf), "BEACON 1919")
	require.Contains(t, string(buf), "<span>810</span>")
	require.Contains(t, string(buf), ">801</code>")
	require.NotContains(t, string(buf), "___")
}

func TestRenderIndex_MissingTemplate(t *testing.T) {
	require.NoError(t, os.Rename("index.src.html", "index.src.html.bak"))
	defer func() {
		require.NoError(t, os.Rename("index.src.html.bak", "index.src.html"))
	}()

	_, err := renderIndex(renderIndexOptions{})
	require.Error(t, err)
}

func TestCreateIndexFromBeaconFile(t *testing.T) {
	t.Setenv("SECRET_KEY", "1145141919810")
	const (
		input  = "beacon.test.txt"
		output = "index.test.html"
	)
	defer func() {
		_ = os.Remove(input)
		_ = os.Remove(output)
	}()

	now := time.Date(2021, 8, 10, 0, 0, 0, 0, time.UTC)
	bct := now.Add(-DisclosureTerm / 2).Format(time.RFC3339)

	require.NoError(t, os.WriteFile(input, []byte(bct), 0644))

	err := createIndexFileFromBeaconFile(now, input, output)
	require.NoError(t, err)

	buf, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(buf), "<em>"+now.Format(time.RFC3339)+"</em>")
	require.Contains(t, string(buf), "status-success")
	require.Contains(t, string(buf), "BEACON ACTIVE")
	require.Contains(t, string(buf), "<span>"+bct+"</span>")
	require.Contains(t, string(buf), ">N/A</code>")

	bct = now.Add(-DisclosureTerm * 2).Format(time.RFC3339)

	require.NoError(t, os.WriteFile(input, []byte(bct), 0644))

	err = createIndexFileFromBeaconFile(now, input, output)
	require.NoError(t, err)

	buf, err = os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(buf), "<em>"+now.Format(time.RFC3339)+"</em>")
	require.Contains(t, string(buf), "status-danger")
	require.Contains(t, string(buf), "BEACON INACTIVE")
	require.Contains(t, string(buf), "<span>"+bct+"</span>")
	require.Contains(t, string(buf), ">1145141919810</code>")
}

func TestCreateIndexFromBeaconFile_MissingInput(t *testing.T) {
	err := createIndexFileFromBeaconFile(time.Now(), "nonexistent.beacon.txt", "index.test.html")
	require.Error(t, err)
}

func TestCreateIndexFromBeaconFile_InvalidBeacon(t *testing.T) {
	const input = "beacon.invalid.test.txt"
	require.NoError(t, os.WriteFile(input, []byte("invalid"), 0644))
	defer func() {
		_ = os.Remove(input)
		_ = os.Remove("index.test.html")
	}()

	err := createIndexFileFromBeaconFile(time.Now(), input, "index.test.html")
	require.Error(t, err)
}

func TestCreateIndexFromBeaconFile_EmptySecretKey(t *testing.T) {
	t.Setenv("SECRET_KEY", "")
	const (
		input  = "beacon.empty.test.txt"
		output = "index.empty.test.html"
	)
	defer func() {
		_ = os.Remove(input)
		_ = os.Remove(output)
	}()

	now := time.Date(2021, 8, 10, 0, 0, 0, 0, time.UTC)
	bct := now.Add(-DisclosureTerm * 2).Format(time.RFC3339)

	require.NoError(t, os.WriteFile(input, []byte(bct), 0644))

	err := createIndexFileFromBeaconFile(now, input, output)
	require.NoError(t, err)

	buf, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(buf), "status-danger")
	require.Contains(t, string(buf), "BEACON INACTIVE")
	require.Contains(t, string(buf), "></code>")
}
