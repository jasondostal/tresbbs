#!/bin/bash
# Build and run the TresBBS playground board.
cd "$(dirname "$0")"
go build -o tresbbs-server ./cmd/tresbbs-server || exit 1
./tresbbs-server -db board.db -scanfiles          # pick up any new files
echo "Starting TresBBS on :2323  (telnet localhost 2323)"
exec ./tresbbs-server -db board.db -addr :2323
