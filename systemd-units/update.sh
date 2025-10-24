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

if ! command -v go &>/dev/null; then
    echo "Go not found in PATH; please install Go or set PATH properly."
    exit 1
fi

if ! command -v git &>/dev/null; then
    echo "git not found in PATH; please install git or set PATH properly."
    exit 1
fi

cd /opt/odamaster/repo
git fetch origin
if ! git diff --quiet HEAD origin/main; then
    echo "New commits detected, rebuilding..."
    git reset --hard origin/main
    go build -o /opt/odamaster/bin/odamaster main.go
else
    echo "No updates."
fi
