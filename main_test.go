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
	require.Contains(t, string(buf), "alert-514")
	require.Contains(t, string(buf), "BEACON 1919")
	require.Contains(t, string(buf), "<span>810</span>")
	require.Contains(t, string(buf), "<code>801</code>")
	require.NotContains(t, string(buf), "___")
}

func TestCreateIndexFromBeaconFile(t *testing.T) {
	os.Setenv("SECRET_KEY", "1145141919810")
	const (
		input  = "beacon.test.txt"
		output = "index.test.html"
	)
	now := time.Date(2021, 8, 10, 0, 0, 0, 0, time.UTC)
	bct := now.Add(-DisclosureTerm / 2).Format(time.RFC3339)

	os.WriteFile(input, []byte(bct), 0644)

	err := createIndexFileFromBeaconFile(now, input, output)
	require.NoError(t, err)

	buf, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(buf), "<em>"+now.Format(time.RFC3339)+"</em>")
	require.Contains(t, string(buf), "alert-success")
	require.Contains(t, string(buf), "BEACON ACTIVE")
	require.Contains(t, string(buf), "<span>"+bct+"</span>")
	require.Contains(t, string(buf), "<code>N/A</code>")

	bct = now.Add(-DisclosureTerm * 2).Format(time.RFC3339)

	os.WriteFile(input, []byte(bct), 0644)

	err = createIndexFileFromBeaconFile(now, input, output)
	require.NoError(t, err)

	buf, err = os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(buf), "<em>"+now.Format(time.RFC3339)+"</em>")
	require.Contains(t, string(buf), "alert-danger")
	require.Contains(t, string(buf), "BEACON INACTIVE")
	require.Contains(t, string(buf), "<span>"+bct+"</span>")
	require.Contains(t, string(buf), "<code>1145141919810</code>")

}
