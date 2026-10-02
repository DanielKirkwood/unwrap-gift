#!/bin/sh
# Backs up the production SQLite database (unwrap-gift's own data --
# wishlists, phone numbers, draw history) using SQLite's own .backup
# command, not a raw file copy: the unwrap-gift-data volume is WAL-mode
# (see internal/db/store.go's dsn()), so a plain cp/tar risks copying an
# inconsistent snapshot mid-write. .backup is safe to run concurrently
# against a live database.
#
# The unwrap-gift container itself has no sqlite3 CLI (it's a scratch-
# based image, built on modernc.org/sqlite -- a pure-Go driver, no system
# binary needed to run the app), so this runs a disposable Alpine
# container instead, mounting the same named volume.
#
# Mounted read-write, not read-only: opening a WAL-mode database (even
# just to read it) can require creating/touching the -wal/-shm sidecar
# files if they're not already present, which a read-only mount would
# block, failing the backup entirely with "unable to open database
# file" (verified by hand). This never modifies application data --
# .backup only reads from the source and writes to the destination.
#
# Usage: ./backup-sqlite.sh [backup-dir]
#   backup-dir defaults to ./backups (relative to this script's directory).
#
# Run daily via cron, e.g.:
#   0 3 * * * cd /path/to/deploy/production && ./backup-sqlite.sh >> backup.log 2>&1
#
# Copy the resulting files off the VPS (rsync/rclone to object storage) --
# a backup that lives on the same disk as the database it backs up isn't
# a backup. See "Postgres backups" in README.md for the equivalent
# Kratos/Keto database backup.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
backup_dir=${1:-"$script_dir/backups"}
volume_name="unwrap-gift-production_unwrap-gift-data"
alpine_image="alpine:3.21"

mkdir -p "$backup_dir"

docker run --rm \
    -v "${volume_name}:/data" \
    -v "${backup_dir}:/backup" \
    "$alpine_image" \
    sh -c "apk add --no-cache sqlite >/dev/null 2>&1 && sqlite3 /data/unwrap-gift.db \".backup /backup/unwrap-gift-\$(date +%F).db\""

echo "Backed up unwrap-gift.db to ${backup_dir}/unwrap-gift-$(date +%F).db"
