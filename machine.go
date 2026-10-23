package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type machineFile struct {
	Path     string `json:"path"`
	Relative string `json:"relative"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256,omitempty"`
}

type machineEvent struct {
	Type       string        `json:"type"`
	Name       string        `json:"name,omitempty"`
	Downloaded int64         `json:"downloaded"`
	Total      int64         `json:"total"`
	Rate       float64       `json:"rate,omitempty"`
	Root       string        `json:"root,omitempty"`
	Files      []machineFile `json:"files,omitempty"`
	Error      string        `json:"error,omitempty"`
}

func emitMachineError(w io.Writer, err error) {
	_ = json.NewEncoder(w).Encode(machineEvent{Type: "error", Error: err.Error()})
}

func isVideo(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mkv", ".mp4", ".m4v", ".avi", ".mov", ".webm", ".ts", ".m2ts", ".mts", ".mpg", ".mpeg", ".wmv", ".flv", ".ogv", ".vob":
		return true
	}
	return false
}

func runMachine(ctx context.Context, input io.Reader, output io.Writer, directory string, videoOnly bool) error {
	return runMachineConfigured(ctx, input, output, directory, videoOnly, nil)
}

func runMachineConfigured(ctx context.Context, input io.Reader, output io.Writer, directory string, videoOnly bool, configure func(*torrent.ClientConfig)) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return fmt.Errorf("no magnet supplied")
	}
	magnet := strings.TrimSpace(scanner.Text())
	parsedMagnet, parseErr := metainfo.ParseMagnetUri(magnet)
	if parseErr != nil {
		return fmt.Errorf("invalid magnet: %w", parseErr)
	}
	if directory == "" {
		directory = "downloads"
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	config := torrent.NewDefaultClientConfig()
	config.DataDir = directory
	config.ListenPort = 0
	config.Debug = false
	var logger log.Logger
	logger.SetHandlers(log.DiscardHandler)
	config.Logger = logger
	if configure != nil {
		configure(config)
	}
	client, err := torrent.NewClient(config)
	if err != nil {
		return err
	}
	defer client.Close()
	t, err := client.AddMagnet(magnet)
	if err != nil {
		return err
	}
	cachePath := filepath.Join(directory, ".torrenter-"+hex.EncodeToString(parsedMagnet.InfoHash[:])+".torrent")
	if cached, err := metainfo.LoadFromFile(cachePath); err == nil && cached.HashInfoBytes() == parsedMagnet.InfoHash {
		// Invalid cached metadata falls back to obtaining it from peers.
		_ = t.SetInfoBytes(cached.InfoBytes)
	}
	encoder := json.NewEncoder(output)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	if err := encoder.Encode(machineEvent{Type: "metadata"}); err != nil {
		return err
	}
	metadataDeadline := time.NewTimer(3 * time.Minute)
	defer metadataDeadline.Stop()
metadata:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-metadataDeadline.C:
			return fmt.Errorf("timed out waiting for torrent metadata")
		case <-ticker.C:
			if err := encoder.Encode(machineEvent{Type: "metadata"}); err != nil {
				return err
			}
		case <-t.GotInfo():
			break metadata
		}
	}
	if err := cacheMetainfo(directory, cachePath, t.Metainfo()); err != nil {
		return fmt.Errorf("caching torrent metadata: %w", err)
	}
	var selected []*torrent.File
	var files []machineFile
	var total int64
	for _, f := range t.Files() {
		if videoOnly && !isVideo(f.Path()) {
			continue
		}
		if !filepath.IsLocal(f.Path()) {
			return fmt.Errorf("unsafe torrent file path")
		}
		if f.Length() <= 0 {
			continue
		}
		selected = append(selected, f)
		files = append(files, machineFile{Path: filepath.Join(directory, f.Path()), Relative: filepath.ToSlash(f.Path()), Size: f.Length()})
		total += f.Length()
	}
	if len(selected) == 0 {
		return fmt.Errorf("torrent contains no matching video files")
	}
	for _, f := range selected {
		f.Download()
	}
	lastBytes, lastTime := int64(0), time.Now()
	for {
		var completed int64
		for _, f := range selected {
			completed += f.BytesCompleted()
		}
		now := time.Now()
		rate := float64(completed-lastBytes) / now.Sub(lastTime).Seconds()
		if lastBytes == 0 && now.Sub(lastTime) < time.Millisecond {
			rate = 0
		}
		if err := encoder.Encode(machineEvent{Type: "progress", Name: t.Name(), Downloaded: completed, Total: total, Rate: rate}); err != nil {
			return err
		}
		lastBytes, lastTime = completed, now
		if completed >= total {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	if err := finalizeMachineFiles(directory, files); err != nil {
		return err
	}
	if err := encoder.Encode(machineEvent{Type: "progress", Name: "Verifying completed videos…", Downloaded: total, Total: total}); err != nil {
		return err
	}
	for i := range files {
		f, err := os.Open(files[i].Path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, hashErr := io.Copy(h, f)
		f.Close()
		if hashErr != nil {
			return hashErr
		}
		files[i].SHA256 = hex.EncodeToString(h.Sum(nil))
	}
	return encoder.Encode(machineEvent{Type: "complete", Name: t.Name(), Downloaded: total, Total: total, Root: directory, Files: files})
}

func cacheMetainfo(directory, destination string, info metainfo.MetaInfo) error {
	f, err := os.CreateTemp(directory, ".torrenter-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := info.Write(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), destination)
}

func finalizeMachineFiles(directory string, files []machineFile) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, f := range files {
		if !filepath.IsLocal(f.Relative) {
			return fmt.Errorf("unsafe file path")
		}
		info, err := root.Lstat(f.Relative)
		if os.IsNotExist(err) {
			if err := root.Rename(f.Relative+".part", f.Relative); err != nil {
				return fmt.Errorf("finalizing %s: %w", f.Relative, err)
			}
			info, err = root.Lstat(f.Relative)
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() != f.Size {
			return fmt.Errorf("completed file has unexpected type or size: %s", f.Relative)
		}
	}
	return nil
}
