package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
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
