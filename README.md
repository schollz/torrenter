# torrenter

install with

```
curl https://gettorrenter.schollz.com | bash 
```

## usage

add a bunch of magnet links to a file:

```
echo 'magnet:...' >> magnet.links
echo 'magnet:...' >> magnet.links
```

and then run

```
torrenter --file magnet.links
```


save the torrent links to a file, e.g.

```
wget https://downloads.raspberrypi.com/raspios_lite_armhf/images/raspios_lite_armhf-2024-07-04/2024-07-04-raspios-bookworm-armhf-lite.img.xz.torrent
```

