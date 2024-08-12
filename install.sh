#!/bin/bash 

ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$ARCH" == "x86_64" ]; then
    ARCH="amd64"
else 
    # exit if not supported
    echo "The architecture $ARCH is not supported."
    exit
fi
if [ "$OS" != "linux" ]; then
    echo "The OS $OS is not supported."
    exit
fi
echo "Downloading torrenter for $OS $ARCH..."
curl -s https://api.github.com/repos/schollz/torrenter/releases/latest | \
    grep 'torrenter_'$OS'_'$ARCH | \
    grep 'browser' | cut -d : -f 2,3 | tr -d \" | wget --show-progress -O torrenter -qi -
chmod +x torrenter
if [ ! -f torrenter ]; then
    echo "Failed to download torrenter."
    exit
else
    sudo mv torrenter /usr/local/bin/torrenter
    echo "Downloaded torrenter to /usr/local/bin/torrenter"
    torrenter --version
fi
