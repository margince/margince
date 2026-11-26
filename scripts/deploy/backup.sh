#!/usr/bin/env bash
# Reference backup: one Postgres dump plus a copy of the object store, taken together,
# into $BACKUP_DIR/<UTC stamp>/. Run it hourly (cron or a systemd timer) to meet the
# one-hour data-loss target. $BACKUP_DIR must lie outside the database and the live
# object store, so a restore cannot roll the evidence back.
#
# Needs: MARGINCE_OWNER_DSN, BACKUP_DIR, and MARGINCE_BLOBSTORE_PATH (object store on disk).
# An S3-compatible store: copy it with your own tool and keep the same folder layout.
set -euo pipefail

: "${MARGINCE_OWNER_DSN:?set the owner DSN}" "${BACKUP_DIR:?set the backup folder}"
: "${MARGINCE_BLOBSTORE_PATH:?set the object store folder}"
RETAIN_DAYS="${RETAIN_DAYS:-14}"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
dest="$BACKUP_DIR/$stamp"
mkdir -p "$dest"

pg_dump --format=custom --dbname="$MARGINCE_OWNER_DSN" --file="$dest/db.dump"
cp -a "$MARGINCE_BLOBSTORE_PATH" "$dest/objects"
# The marker is written last: a folder without it is an unfinished backup.
printf '{"taken_at":"%s"}\n' "$stamp" >"$dest/DONE"

find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -mtime "+$RETAIN_DAYS" -exec rm -rf {} +
echo "$dest"
