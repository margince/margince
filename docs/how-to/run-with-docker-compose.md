<!-- prose:plain -->
# Run Margince with Docker Compose

Run every part of Margince on one host of your own with Docker Compose. Each release ships
`margince-<tag>-compose.tar.gz`. It holds PostgreSQL with pgvector, Redis, the api, the worker, the
web app and an nginx proxy. Each Margince image in it is pinned to its hash. The database is set up on its first
start, and the proxy routes one host to the api and the web app. To run the images on a platform of
your own instead, read [deploy-margince.md](deploy-margince.md).

## Download it and check it

```sh
VERSION=v1.0.0
BASE=https://github.com/margince/margince/releases/download/$VERSION
curl -fLO "$BASE/margince-$VERSION-compose.tar.gz"
curl -fLO "$BASE/SHA256SUMS"
curl -fLO "$BASE/SHA256SUMS.cosign.bundle"
sha256sum -c SHA256SUMS --ignore-missing
cosign verify-blob SHA256SUMS --bundle SHA256SUMS.cosign.bundle \
  --certificate-identity "https://github.com/margince/margince/.github/workflows/release-tag.yml@refs/tags/$VERSION" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
tar -xzf "margince-$VERSION-compose.tar.gz"
cd "margince-$VERSION-compose"
```

## Write the settings

```sh
cp .env.example .env
```

Fill in `.env`. Each line says what goes there, and `docker compose up` names any that is still
empty. Make the passwords with `openssl rand -hex 24` and the vault root key with
`openssl rand -base64 32`. The vault root key seals every secret the app stores, so store it apart
from the data: [back-up-an-installation.md](back-up-an-installation.md).

Edit `margince.yaml` as [Write the first-boot config](deploy-margince.md#write-the-first-boot-config)
says. Its `password_file` already points where the api writes the first admin's password. Any other
setting goes in a file named `margince.env` in this folder, one `NAME=value` on each line; every
setting is in [configuration.md](../reference/configuration.md).

## Start it

```sh
docker compose up -d
curl -fsS http://localhost:8080/readyz
```

The api applies the migrations, then serves; the worker restarts until it has. The proxy listens on
the port `.env` names, 8080 by default. Put a TLS proxy before it, and set `MARGINCE_PUBLIC_BASE_URL`
to the `https` address. Sign in as the `bootstrap_admin` of `margince.yaml`, then remove
`MARGINCE_ADMIN_PASSWORD` from `.env`, as [After the first boot](deploy-margince.md#after-the-first-boot)
says.

## Upgrade to a new release

Download and check the new release's folder as above. Copy `.env`, `margince.yaml` and
`margince.env` into it, then run `docker compose up -d` there. The project name stays
`margince-compose`, so the stack keeps its data.
