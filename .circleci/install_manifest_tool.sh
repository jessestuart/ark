#!/bin/sh
echo "Downloading manifest-tool."
MT_VERSION=$(curl https://api.github.com/repos/estesp/manifest-tool/releases | jq '.[].tag_name' -r | head -n1)
wget https://github.com/estesp/manifest-tool/releases/download/${MT_VERSION}/manifest-tool-linux-amd64
mv manifest-tool-linux-amd64 /usr/bin/manifest-tool
chmod +x /usr/bin/manifest-tool
manifest-tool --version
