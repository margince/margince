#!/bin/sh
# The PostgreSQL image runs this once, when the database volume is empty.
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" \
	-v owner_pw="$MARGINCE_OWNER_PW" -v app_pw="$MARGINCE_APP_PW" \
	-f /margince/db-bootstrap.sql
