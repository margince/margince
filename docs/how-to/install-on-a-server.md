<!-- prose:plain -->
# Install Margince on a server

Run a release on one Linux server of your own, with no containers. Each
release ships a bundle for `linux/amd64` and one for `linux/arm64`. A bundle holds the three programs,
the web app, and the systemd and nginx files below. To run the container images instead, read
[deploy-margince.md](deploy-margince.md).

The two ways run the same build. The settings, the database roles, the license token and the first-boot
config are the same. For those, this page links to [deploy-margince.md](deploy-margince.md).

## What the server needs

- Linux with systemd, on `amd64` or `arm64`.
- PostgreSQL with the pgvector extension, at the version `docker-compose.dev.yml` runs.
- Redis, at the version `docker-compose.dev.yml` runs.
- nginx, to serve the web app and send the api's paths to the api.
- What each other feature needs, such as an object store or a mail relay:
  [What each feature needs](../reference/what-each-feature-needs.md).

## Download the bundle and check it

Take the files from the release page of the version you want:

```sh
VERSION=v1.0.0
ARCH=amd64   # or arm64
BASE=https://github.com/margince/margince/releases/download/$VERSION
curl -fLO "$BASE/margince-$VERSION-linux-$ARCH.tar.gz"
curl -fLO "$BASE/SHA256SUMS"
curl -fLO "$BASE/SHA256SUMS.cosign.bundle"
```

Check that the bundle is the one the release built, then that the release signed `SHA256SUMS`:

```sh
sha256sum -c SHA256SUMS --ignore-missing
cosign verify-blob SHA256SUMS --bundle SHA256SUMS.cosign.bundle \
  --certificate-identity "https://github.com/margince/margince/.github/workflows/release-tag.yml@refs/tags/$VERSION" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Open it

Each release gets its own folder, and `/opt/margince` points at the one that runs. The units and the
nginx files name `/opt/margince`, so an upgrade only moves that link.

```sh
sudo tar -xzf "margince-$VERSION-linux-$ARCH.tar.gz" -C /opt
sudo ln -sfn "/opt/margince-$VERSION-linux-$ARCH" /opt/margince
cat /opt/margince/VERSION
/opt/margince/bin/margince-api --version
sudo useradd --system --user-group --home-dir /var/lib/margince --create-home --shell /usr/sbin/nologin margince
```

`VERSION` and `--version` name the release and the commit it was built from. Each of the three
programs answers `--version`.

## Set up the database once

Create the two database roles, the database and the extensions as a PostgreSQL superuser, with
`scripts/deploy/db-bootstrap.sql`. The bundle carries it in its `deploy` folder:

```sh
psql "postgres://postgres:…@<host>:5432/postgres" \
  -v owner_pw="$OWNER_PW" -v app_pw="$APP_PW" \
  -f /opt/margince/deploy/db-bootstrap.sql
```

The reasons are in [Bootstrap the database once](deploy-margince.md#bootstrap-the-database-once).

## Write the settings

The units read every setting from `/etc/margince/margince.env`, one `NAME=value` on each line. Start
from the template in the bundle:

```sh
sudo install -d -m 0750 -o root -g margince /etc/margince
sudo install -m 0640 -o root -g margince /opt/margince/margince.env.example /etc/margince/margince.env
```

Set [the least production needs](deploy-margince.md#the-least-production-needs). With nginx on the
same server, also set these two:

```sh
MARGINCE_ADDR=127.0.0.1:8080
MARGINCE_TRUSTED_PROXIES=127.0.0.1/32
```

For the first boot, start from the bundle's copy of `config/margince.example.yaml`, and point
`MARGINCE_CONFIG` at your copy:

```sh
sudo install -m 0640 -o root -g margince /opt/margince/config/margince.example.yaml /etc/margince/margince.yaml
echo 'MARGINCE_CONFIG=/etc/margince/margince.yaml' | sudo tee -a /etc/margince/margince.env
```

Follow
[Write the first-boot config](deploy-margince.md#write-the-first-boot-config). The api starts in
`/var/lib/margince`, so `password_file: secrets/admin-password` names
`/var/lib/margince/secrets/admin-password`.

## Start the api and the worker

```sh
sudo cp /opt/margince/deploy/server/margince-api.service /opt/margince/deploy/server/margince-worker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now margince-api
sudo systemctl enable --now margince-worker
journalctl -u margince-api -f
```

The api applies the migrations as the owner role, then serves as the app role, as the api image does.
The worker restarts until the api has recorded the release, which is expected on the first boot.

## Serve it with nginx

The bundle has two nginx files. `web.conf` is the web image's own server, on `127.0.0.1:8081`, for the
`web/` folder. `nginx.conf` takes every request: it sends the api's paths to the api and the rest to
`web.conf`.

```sh
sudo cp /opt/margince/deploy/server/web.conf /etc/nginx/conf.d/margince-web.conf
sudo cp /opt/margince/deploy/server/nginx.conf /etc/nginx/conf.d/margince.conf
sudo nano /etc/nginx/conf.d/margince.conf   # set server_name to your host
sudo nginx -t && sudo systemctl reload nginx
```

Then add the certificate, for example with `sudo certbot --nginx -d <your host>`. Set
`MARGINCE_PUBLIC_BASE_URL` to the `https` address, and restart the api and the worker.

## Check it

```sh
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS https://<your host>/v1/status
```

`/readyz` answers 200 once Postgres, Redis and every other set dependency answers. The health checks
are in [Point the health checks](deploy-margince.md#point-the-health-checks).

## Upgrade to a new release

Download, check and open the new bundle as above. Then move the link and restart in the order the
release guard needs: stop the worker, start the api, then start the worker.

```sh
sudo systemctl stop margince-worker
sudo ln -sfn "/opt/margince-$VERSION-linux-$ARCH" /opt/margince
sudo cp /opt/margince/deploy/server/margince-api.service /opt/margince/deploy/server/margince-worker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl restart margince-api
sudo systemctl start margince-worker
```

The copies under `/etc` do not follow the link, so take the units again as above. Take `web.conf` again
too. Compare the new `nginx.conf` with yours, which holds your host name and certificate, and carry over
what changed:

```sh
sudo cp /opt/margince/deploy/server/web.conf /etc/nginx/conf.d/margince-web.conf
diff /opt/margince/deploy/server/nginx.conf /etc/nginx/conf.d/margince.conf
sudo nginx -t && sudo systemctl reload nginx
```

The api applies the new release's migrations when it starts. Keep the folder of the release before
until the new one is serving. The order, and what a worker at the wrong release does, is in
[Roll out in this order](deploy-margince.md#roll-out-in-this-order).
