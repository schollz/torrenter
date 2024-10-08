package main

import (
	"bufio"
	"flag"
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

var Version string

func main() {
	// Define a command-line flag for the magnet links file
	filename := flag.String("file", "magnet.links", "Path to the magnet links file")
	version := flag.Bool("version", false, "Print the version of the program")
	flag.Parse()

	if *version {
		fmt.Println("torrenter " + Version)
		os.Exit(0)
	}

	// Check if the specified magnet file exists
	if _, err := os.Stat(*filename); os.IsNotExist(err) {
		fmt.Printf("file %s not found\n", filename)
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
	// disable error messages
	config.Debug = false

	// Create a torrent client
	client, err := torrent.NewClient(config)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	// setup progressbar
	var wg sync.WaitGroup
	p := mpb.New(mpb.WithWaitGroup(&wg))
	numTorrents := 0

	// check if .torrent file
	if strings.HasSuffix(*filename, ".torrent") {
		numTorrents = 1
		wg.Add(1)
		// Add the torrent from the torrent file
		t, err := client.AddTorrentFromFile(*filename)
		if err != nil {
			return
		}

		// Wait for the torrent to get metadata
		<-t.GotInfo()

		// Start downloading the torrent
		go showDownloadProgress(t, p, &wg)
		time.Sleep(1 * time.Second)
	} else {
		// Open the magnet file and start downloading
		file, err := os.Open(*filename)
		if err != nil {
			panic(err)
		}
		defer file.Close()

		// Open file and count the number of valid magnet links
		fileContents, err := os.ReadFile(*filename)
		if err != nil {
			panic(err)
		}
		for _, line := range strings.Split(string(fileContents), "\n") {
			if strings.HasPrefix(line, "magnet:") {
				numTorrents++
			}
		}
		if numTorrents == 0 {
			return
		}
		fmt.Printf("Downloading %d torrents\n", numTorrents)
		wg.Add(numTorrents)

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
			go showDownloadProgress(t, p, &wg)
			time.Sleep(1 * time.Second)
		}

		if err := scanner.Err(); err != nil {
			panic(err)
		}
	}

	if numTorrents == 0 {
		return
	}
	p.Wait()

}

func showDownloadProgress(t *torrent.Torrent, p *mpb.Progress, wg *sync.WaitGroup) {
	t.DownloadAll()

	if t.BytesMissing() == 0 {
		fmt.Printf("[%s] done\n", t.Name())
	} else {
		bar := p.AddBar(int64(t.BytesMissing()),
			mpb.PrependDecorators(
				// Simple name decorator
				decor.Name(fmt.Sprintf("[%s]", t.Name())),
				// Decor.DSyncWidth bit enables column width synchronization
				decor.Percentage(decor.WCSyncSpace),
			),
			mpb.AppendDecorators(
				decor.EwmaSpeed(decor.SizeB1024(0), "% .1f ", 60),
				// Replace ETA decorator with "done" message, OnComplete event
				decor.OnComplete(
					// ETA decorator with ewma age of 30
					decor.EwmaETA(decor.ET_STYLE_GO, 30, decor.WCSyncWidth), " done",
				),
			),
		)
		for t.BytesMissing() > 0 {
			status := t.Stats()
			time.Sleep(100 * time.Millisecond)
			bar.EwmaSetCurrent(int64(status.BytesRead.Int64()), 10*time.Millisecond)
		}

	}
	wg.Done()
}
