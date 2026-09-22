#!/bin/sh
set -e
go build -o /Users/felix/bin/remapper .
go build -o /Users/felix/bin/tilepicker ./tilepicker
go build -o /Users/felix/bin/sfxmapper ./sfxmapper
