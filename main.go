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

	"github.com/anacrolix/log"
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
		fmt.Printf("file %s not found\n", *filename)
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
	config.Debug = false
	var logger log.Logger
	logger.SetHandlers(log.DiscardHandler)
	config.Logger = logger

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
		go showDownloadProgress(t, p, &wg, downloadDirectory)
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
			go showDownloadProgress(t, p, &wg, downloadDirectory)
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

func showDownloadProgress(t *torrent.Torrent, p *mpb.Progress, wg *sync.WaitGroup, dataDir string) {
	t.DownloadAll()

	if t.BytesMissing() == 0 {
		fmt.Printf("[%s] done\n", t.Name())
	} else {
		totalBytes := t.BytesMissing()
		bar := p.AddBar(totalBytes,
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
			downloaded := totalBytes - t.BytesMissing()
			time.Sleep(100 * time.Millisecond)
			bar.EwmaSetCurrent(downloaded, 100*time.Millisecond)
		}
		bar.SetCurrent(totalBytes)
	}

	// Promote any remaining .part files to their final names. The library
	// renames .part → final inside MarkComplete(), but errors there are
	// only logged while the piece is still marked complete, so we do a
	// defensive sweep here after the download reports as finished.
	promotePartFiles(t, dataDir)

	wg.Done()
}

// promotePartFiles renames any leftover .part files to their final paths.
func promotePartFiles(t *torrent.Torrent, dataDir string) {
	for _, f := range t.Files() {
		partPath := filepath.Join(dataDir, f.Path()+".part")
		finalPath := filepath.Join(dataDir, f.Path())
		if _, err := os.Stat(partPath); err != nil {
			continue // no .part file, nothing to do
		}
		if _, err := os.Stat(finalPath); err == nil {
			continue // final file already exists
		}
		if err := os.Rename(partPath, finalPath); err != nil {
			fmt.Printf("warning: could not rename %s → %s: %v\n", partPath, finalPath, err)
		}
	}
}
