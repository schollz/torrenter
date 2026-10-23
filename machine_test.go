package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func TestVideoExtensions(t *testing.T) {
	for _, name := range []string{"a.mkv", "a.MP4", "season/ep02.webm", "movie.ts", "film.m2ts"} {
		if !isVideo(name) {
			t.Errorf("rejected video %s", name)
		}
	}
	for _, name := range []string{"readme.txt", "subtitles.srt", "poster.jpg", "movie.mkv.exe", "info.nfo"} {
		if isVideo(name) {
			t.Errorf("accepted non-video %s", name)
		}
	}
}

func TestMachineDownloadsMultipleVideosFromLocalPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	source := t.TempDir()
	season := filepath.Join(source, "Season")
	if err := os.MkdirAll(season, 0755); err != nil {
		t.Fatal(err)
	}
	contents := map[string][]byte{"one.mkv": bytes.Repeat([]byte("video-one"), 8192), "two.mp4": bytes.Repeat([]byte("video-two"), 4096), "readme.txt": []byte("not a video")}
	for name, b := range contents {
		if err := os.WriteFile(filepath.Join(season, name), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	info := metainfo.Info{PieceLength: 16 * 1024}
	if err := info.BuildFromFilePath(season); err != nil {
		t.Fatal(err)
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mi := &metainfo.MetaInfo{InfoBytes: infoBytes}
	configure := func(c *torrent.ClientConfig) {
		c.NoDHT = true
		c.DisableTrackers = true
		c.DisableUTP = true
		c.DisableIPv6 = true
		c.NoDefaultPortForwarding = true
		c.ListenHost = func(string) string { return "127.0.0.1" }
	}
	config := torrent.NewDefaultClientConfig()
	config.DataDir = source
	config.ListenPort = 0
	config.Seed = true
	configure(config)
	seeder, err := torrent.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer seeder.Close()
	seed, err := seeder.AddTorrent(mi)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.VerifyDataContext(ctx); err != nil {
		t.Fatal(err)
	}
	magnet := mi.Magnet(nil, &info)
	magnet.Params.Set("x.pe", seeder.ListenAddrs()[0].String())
	var events bytes.Buffer
	dest := t.TempDir()
	if err := runMachineConfigured(ctx, strings.NewReader(magnet.String()+"\n"), &events, dest, true, configure); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&events)
	var final machineEvent
	for decoder.More() {
		var event machineEvent
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "complete" {
			final = event
		}
	}
	if len(final.Files) != 2 || final.Total == 0 || final.Downloaded != final.Total {
		t.Fatalf("incorrect completion event: %+v", final)
	}
	for _, f := range final.Files {
		actual, err := os.ReadFile(f.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, contents[filepath.Base(f.Relative)]) {
			t.Fatal("video bytes differ")
		}
		if f.SHA256 != fmt.Sprintf("%x", sha256.Sum256(actual)) {
			t.Fatal("missing or incorrect video checksum")
		}
		if !isVideo(f.Relative) {
			t.Fatal("non-video in manifest")
		}
	}
	seed.Drop()
	var resumed bytes.Buffer
	if err := runMachineConfigured(ctx, strings.NewReader(magnet.String()+"\n"), &resumed, dest, true, configure); err != nil {
		t.Fatalf("could not resume cached download without a seeder: %v", err)
	}
}

func TestMachineRejectsInvalidInput(t *testing.T) {
	var output bytes.Buffer
	if err := runMachine(context.Background(), bytes.NewBufferString("bad magnet\n"), &output, t.TempDir(), true); err == nil {
		t.Fatal("accepted invalid magnet")
	}
	if output.Len() != 0 {
		t.Fatal("invalid input started a download")
	}
}

func TestFinalizeOnlyManifestFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Show"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Show/ep.mkv.part"), []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt.part"), []byte("info"), 0644); err != nil {
		t.Fatal(err)
	}
	files := []machineFile{{Relative: "Show/ep.mkv", Size: 5}}
	if err := finalizeMachineFiles(dir, files); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Show/ep.mkv")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "readme.txt")); !os.IsNotExist(err) {
		t.Fatal("promoted unrelated text file")
	}
	if err := finalizeMachineFiles(dir, []machineFile{{Relative: "../escape.mkv", Size: 5}}); err == nil {
		t.Fatal("accepted path traversal")
	}
	if err := finalizeMachineFiles(dir, []machineFile{{Relative: "Show/ep.mkv", Size: 6}}); err == nil {
		t.Fatal("accepted truncated file")
	}
}
