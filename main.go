package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

func main() {
	// Check if 'magnet.links' file exists
	if _, err := os.Stat("magnet.links"); os.IsNotExist(err) {
		fmt.Println("magnet.links file not found")
		os.Exit(1)
	}

	// Create a torrent client config and set download directory
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	downloadDirectory := filepath.Join(cwd, "downloads")
	config := torrent.NewDefaultClientConfig()
	config.DataDir = downloadDirectory

	// Create a torrent client
	client, err := torrent.NewClient(config)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	// Open the magnet.links file and start downloading
	file, err := os.Open("magnet.links")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// open file and count the number of valid magnet links
	fileConents, err := os.ReadFile("magnet.links")
	if err != nil {
		panic(err)
	}
	numMagnetLinks := 0
	for _, line := range strings.Split(string(fileConents), "\n") {
		if strings.HasPrefix(line, "magnet:") {
			numMagnetLinks++
		}
	}

	var wg sync.WaitGroup
	// passed wg will be accounted at p.Wait() call
	p := mpb.New(mpb.WithWaitGroup(&wg))
	numBars := numMagnetLinks
	wg.Add(numBars)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		link := scanner.Text()
		// Add the torrent from magnet link
		t, err := client.AddMagnet(link)
		if err != nil {
			fmt.Printf("Error adding magnet link: %v\n", err)
			continue
		}

		// Wait for the torrent to get metadata
		<-t.GotInfo()

		// Start downloading the torrent
		go func(t *torrent.Torrent) {
			t.DownloadAll()

			bar := p.AddBar(int64(t.BytesMissing()),
				mpb.PrependDecorators(
					// simple name decorator
					decor.Name(t.Name()),
					// decor.DSyncWidth bit enables column width synchronization
					decor.Percentage(decor.WCSyncSpace),
				),
				mpb.AppendDecorators(
					decor.EwmaSpeed(decor.SizeB1024(0), "% .1f", 60),
					// replace ETA decorator with "done" message, OnComplete event
					decor.OnComplete(
						// ETA decorator with ewma age of 30
						decor.EwmaETA(decor.ET_STYLE_GO, 30, decor.WCSyncWidth), " done",
					),
				),
			)

			for t.BytesMissing() > 0 {
				status := t.Stats()
				time.Sleep(10 * time.Millisecond)
				bar.EwmaSetCurrent(int64(status.BytesRead.Int64()), 10*time.Millisecond)
			}
			wg.Done()
		}(t)
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	// Wait indefinitely to allow downloads to complete
	p.Wait()
}
