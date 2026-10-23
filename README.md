# torrenter

install with

```
curl https://gettorrenter.schollz.com | bash 
```

## usage


### magnet links

add a bunch of magnet links to a file:

```
echo 'magnet:...' >> magnet.links
echo 'magnet:...' >> magnet.links
```

and then run

```
torrenter --file magnet.links
```

### torrent links

save the torrent links to a file, e.g.

```
wget https://downloads.raspberrypi.com/raspios_lite_armhf/images/raspios_lite_armhf-2024-07-04/2024-07-04-raspios-bookworm-armhf-lite.img.xz.torrent
torrenter --file 2024-07-04-raspios-bookworm-armhf-lite.img.xz.torrent
```

## license

## JSON mode for remote clients

Download one magnet supplied on stdin and emit newline-delimited JSON events:

```sh
printf '%s\n' "$MAGNET" | torrenter --json --magnet-stdin --video-only --dir downloads/job
torrenter --protocol-version # prints 1
```

Events are `metadata`, `progress`, `complete`, and `error`. Progress reports `downloaded`, `total`, and `rate` in bytes and bytes/second. The final `complete` event includes an absolute `root` and a `files` manifest with absolute `path`, slash-separated `relative` path, and `size` in bytes. Nonzero exit status indicates failure. No terminal control sequences are written in JSON mode.

`--video-only` selects all video containers, including multiple episodes, and excludes text, subtitles, images, and archives. Existing data is checked and reused on subsequent runs. Interrupting the process preserves downloaded data. A broken JSON output pipe stops the client. Normal `--file` usage retains its terminal progress interface; `--dir` can also override its download directory.

Builds require Go 1.26 or newer after the dependency upgrade. CI reads the required version from `go.mod`. The latest `stm` and `missinggo/v2` releases currently break the torrent library's APIs, so those two modules remain at the newest compatible versions, v0.5.0 and v2.10.0 respectively.

## license

MIT

