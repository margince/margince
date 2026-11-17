#!/usr/bin/env bash
# Run this checkout the way a customer runs a release: production mode, through
# the Docker Compose folder a release ships, with the images built from here.
#
#   scripts/prod.sh up            build the images, then start the stack
#   scripts/prod.sh stop [--drop] stop it; --drop also deletes its data
#
# It is not `make dev`. Production mode holds every check a real install holds
# and starts the 14-day trial when no license token is set. The stack keeps its
# data and settings under .tmp/production/ between runs. .env there is written
# once, with new secrets; put MARGINCE_LICENSE in it to end the trial.
#
# PROD_PORT picks the port on this machine (8090 by default, apart from the
# dev stack's 8080). The stack listens on 127.0.0.1 only and serves plain HTTP,
# so it is for this machine alone.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
state="$root/.tmp/production"
stack="$state/stack"
port="${PROD_PORT:-8090}"

# project is one Compose project per settings folder. Its ID is drawn once and
# kept in the folder, so a folder shared by a symlink shares its stack.
project() {
	mkdir -p "$state"
	if [[ ! -s "$state/project-id" ]]; then
		# ln fails when the name exists, so of two runs at once the first ID wins.
		local draft
		draft="$(mktemp "$state/project-id.XXXXXX")"
		openssl rand -hex 8 >"$draft"
		ln "$draft" "$state/project-id" 2>/dev/null || true
		rm -f "$draft"
	fi
	printf 'margince-prod-%s' "$(cat "$state/project-id")"
}

# compose runs with MARGINCE_ENV empty, because a value exported in the
# caller's shell outranks the stack's .env and would start it in dev mode.
compose() {
	MARGINCE_ENV= docker compose --project-directory "$stack" --project-name "$(project)" "$@"
}

# write_env: the stack's settings, made once so a second run keeps its data.
# A later run keeps the secrets and moves only the port: PROD_PORT when given,
# else the port saved last time.
write_env() {
	if [[ -f "$state/.env" ]]; then
		if [[ -z "${PROD_PORT:-}" ]]; then
			port="$(sed -n 's/^MARGINCE_PORT=127\.0\.0\.1://p' "$state/.env")"
		fi
		sed -i.bak -e "s|^MARGINCE_PORT=.*|MARGINCE_PORT=127.0.0.1:$port|" \
			-e "s|^MARGINCE_PUBLIC_BASE_URL=.*|MARGINCE_PUBLIC_BASE_URL=http://localhost:$port|" "$state/.env"
		rm -f "$state/.env.bak"
		return
	fi
	umask 077
	{
		echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)"
		echo "MARGINCE_OWNER_PW=$(openssl rand -hex 24)"
		echo "MARGINCE_APP_PW=$(openssl rand -hex 24)"
		echo "MARGINCE_PUBLIC_BASE_URL=http://localhost:$port"
		echo "MARGINCE_KEYVAULT_ROOT_KEY=$(openssl rand -base64 32)"
		echo "MARGINCE_ADMIN_PASSWORD=$(openssl rand -hex 12)"
		echo "MARGINCE_PORT=127.0.0.1:$port"
		echo "MARGINCE_LICENSE="
		echo "MARGINCE_ENV="
	} >"$state/.env"
}

up() {
	local tag
	tag="local-$(git -C "$root" rev-parse --short HEAD)"
	mkdir -p "$state"
	echo "prod: building the api, web and worker images ($tag)"
	(cd "$root" && REPO=margince-local VERSION="$tag" \
		MARGINCE_BUILD_REVISION="$(git rev-parse HEAD)" \
		docker buildx bake api web worker --load)
	bash "$root/scripts/release-compose.sh" --local "$tag" "$stack"
	write_env
	cp "$state/.env" "$stack/.env"
	compose up -d
	for _ in $(seq 120); do
		if curl -fsS "http://localhost:$port/readyz" >/dev/null 2>&1; then
			report
			return
		fi
		sleep 2
	done
	echo "prod: the stack did not answer on http://localhost:$port/readyz." >&2
	compose logs --no-color --tail 60 api worker front >&2
	exit 1
}

report() {
	local password
	password="$(grep -E '^MARGINCE_ADMIN_PASSWORD=' "$state/.env" | cut -d= -f2-)"
	echo "prod: Margince runs on http://localhost:$port in production mode."
	echo "  Sign in as admin@demo.test with the password $password (on the first start only)."
	echo "  With no MARGINCE_LICENSE in $state/.env it runs the 14-day trial."
	echo "  Stop it with \`make prod-stop\`; DROP=1 also deletes its data."
}

stop() {
	if [[ ! -f "$stack/compose.yaml" ]]; then
		echo "prod: no stack under $stack, so nothing to stop."
		return
	fi
	if [[ "${1:-}" == "--drop" ]]; then
		compose down --volumes --remove-orphans
		# A settings folder shared through a symlink is deleted where it lives.
		rm -rf "$(cd "$state" && pwd -P)"
		rm -f "$state"
		echo "prod: stopped, and its data and settings are deleted."
		return
	fi
	compose down --remove-orphans
	echo "prod: stopped. Its data stays for the next \`make prod\`."
}

case "${1:-}" in
up) up ;;
stop) stop "${2:-}" ;;
*)
	echo "usage: scripts/prod.sh up | stop [--drop]" >&2
	exit 2
	;;
esac
