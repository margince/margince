#!/usr/bin/env bash
# Reference restore of a backup made by backup.sh into a SPARE database and object
# store, then the erasure replay. Never point it at the live installation.
#
# usage: restore.sh <backup folder>     (e.g. $BACKUP_DIR/20261111T080000Z)
# Needs: SPARE_OWNER_DSN, SPARE_BLOBSTORE_PATH, BACKUP_DIR, the worker settings for the
# spare stack (MARGINCE_DSN, MARGINCE_BLOBSTORE_PATH, MARGINCE_KEYVAULT_ROOT_KEY), and
# LIVE_BLOBSTORE_PATH, the live object store that holds the newest erasure journal.
set -euo pipefail

src="${1:?usage: restore.sh <backup folder>}"
: "${SPARE_OWNER_DSN:?}" "${SPARE_BLOBSTORE_PATH:?}" "${BACKUP_DIR:?}" "${LIVE_BLOBSTORE_PATH:?}"
[ -f "$src/DONE" ] || { echo "refusing: $src has no DONE marker (unfinished backup)" >&2; exit 1; }

started=$(date +%s)
pg_restore --clean --if-exists --no-owner --dbname="$SPARE_OWNER_DSN" "$src/db.dump"
mkdir -p "$SPARE_BLOBSTORE_PATH"
cp -a "$src/objects/." "$SPARE_BLOBSTORE_PATH/"

# The backup's journal is older than the live one; the replay needs the newest.
rm -rf "$SPARE_BLOBSTORE_PATH/erasure-suppression"
cp -a "$LIVE_BLOBSTORE_PATH/erasure-suppression" "$SPARE_BLOBSTORE_PATH/erasure-suppression"

margince-migrate up --dsn "$SPARE_OWNER_DSN"
# A failure here stops the script: do not let users back in without the replay.
worker replay-erasures

seconds=$(( $(date +%s) - started ))
# Evidence outside the restored data: the database ledger rolls back with a restore.
printf '%s restored %s in %ss\n' "$(date -u +%FT%TZ)" "$(basename "$src")" "$seconds" >>"$BACKUP_DIR/restores.log"
echo "restored in ${seconds}s; replay done. Anything erased after the journal's newest time is still yours to erase by hand."
