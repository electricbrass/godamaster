#!/bin/bash

# Copyright (C) 2025 Mia McMahill
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation; either version 2 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)

if ! command -v go &>/dev/null; then
    echo "Go not found in PATH; please install Go or set PATH properly."
    exit 1
fi

if ! command -v git &>/dev/null; then
    echo "git not found in PATH; please install git or set PATH properly."
    exit 1
fi

if ! id -u odamaster &>/dev/null; then
    sudo useradd -r -s /usr/sbin/nologin -d /opt/odamaster odamaster
fi
sudo install -m 0700 -o odamaster -g odamaster -d /opt/odamaster
sudo install -m 0700 -o odamaster -g odamaster -d /opt/odamaster/bin
sudo install -T -m 644 -o root -g root "$script_dir/systemd-units/odamaster.service"        "/etc/systemd/system/odamaster.service"
sudo install -T -m 644 -o root -g root "$script_dir/systemd-units/odamaster-update.service" "/etc/systemd/system/odamaster-update.service"
sudo install -T -m 644 -o root -g root "$script_dir/systemd-units/odamaster-update.timer"   "/etc/systemd/system/odamaster-update.timer"
sudo install -T -m 500 -o odamaster -g odamaster "$script_dir/systemd-units/update.sh" "/opt/odamaster/bin/update.sh"
if [[ -d /opt/odamaster/repo/.git ]]; then
    sudo -u odamaster git -C /opt/odamaster/repo fetch origin
    sudo -u odamaster git -C /opt/odamaster/repo reset --hard origin/main
else
    sudo -u odamaster git clone https://github.com/electricbrass/godamaster.git /opt/odamaster/repo
fi
sudo -u odamaster go build -C /opt/odamaster/repo -o /opt/odamaster/bin/odamaster
sudo systemctl daemon-reload
sudo systemctl enable --now odamaster-update.timer
sudo systemctl enable --now odamaster.service
