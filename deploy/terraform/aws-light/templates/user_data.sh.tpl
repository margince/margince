#!/bin/bash
# Boot-time provisioning for the single EC2 instance this stack creates.
# Runs once, at first boot (cloud-init), as root. Idempotent enough to
# survive a `systemctl restart margince` or a reboot re-running it, but NOT
# re-run automatically on every boot after the first — the systemd unit
# this writes is what starts the stack on every subsequent boot, not this
# script.
set -euo pipefail

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  COMPOSE_ARCH="x86_64" ;;
  aarch64) COMPOSE_ARCH="aarch64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

dnf install -y docker awscli2 postgresql15
systemctl enable --now docker
# postgresql15 (client only, psql) — this stack's README uses this SAME
# instance, over SSM, to run scripts/deploy/db-bootstrap.sql against RDS
# once, since there's no separate bastion here the way the full stack
# documents.

# AL2023's docker package ships the engine only, no compose plugin — this
# is the pinned static binary Docker publishes for exactly this case,
# dropped into the cli-plugins directory docker itself scans.
mkdir -p /usr/libexec/docker/cli-plugins
curl -fsSL "https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-$COMPOSE_ARCH" \
  -o /usr/libexec/docker/cli-plugins/docker-compose
chmod +x /usr/libexec/docker/cli-plugins/docker-compose

mkdir -p /opt/margince/config
chmod 700 /opt/margince

%{ if enable_tls ~}
# ---- TLS bootstrap: a throwaway self-signed cert so nginx can bind :443 --
# before Certbot ever runs. nginx refuses to start with an ssl_certificate
# directive pointing at a file that doesn't exist — this dummy cert is
# replaced by the real one a few steps down, the same bootstrap dance
# certbot's own docs recommend for exactly this chicken-and-egg problem.
mkdir -p "/opt/margince/certbot/www" "/opt/margince/certbot/conf/live/${domain}"
if [ ! -f "/opt/margince/certbot/conf/live/${domain}/fullchain.pem" ]; then
  openssl req -x509 -nodes -newkey rsa:2048 -days 1 \
    -keyout "/opt/margince/certbot/conf/live/${domain}/privkey.pem" \
    -out "/opt/margince/certbot/conf/live/${domain}/fullchain.pem" \
    -subj "/CN=${domain}"
fi
%{ endif ~}

# ---- Config object + RDS CA bundle ------------------------------------------
aws s3 cp "s3://${blobstore_bucket}/config/margince.yaml" /opt/margince/config/margince.yaml --region "${aws_region}"
curl -fsSL https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem \
  -o /opt/margince/config/rds-ca-bundle.pem

# ---- Secrets -> .env (mode 600, never written to a log or S3) ---------------
ENV_FILE=/opt/margince/.env
umask 077
: > "$ENV_FILE"
%{ for s in secrets ~}
echo "${s.env_name}=$(aws secretsmanager get-secret-value --secret-id '${s.secret_id}' --region "${aws_region}" --query SecretString --output text)" >> "$ENV_FILE"
%{ endfor ~}
chmod 600 "$ENV_FILE"

# ---- nginx reverse-proxy config (mirrors the full stack's ALB listener rules) --
base64 -d > /opt/margince/nginx.conf <<'NGINX_CONF_B64'
${nginx_conf_b64}
NGINX_CONF_B64

# ---- ECR login + image pull --------------------------------------------------
aws ecr get-login-password --region "${aws_region}" | docker login --username AWS --password-stdin "${ecr_registry}"
docker pull "${ecr_api_url}:${image_tag}"
docker pull "${ecr_worker_url}:${image_tag}"
docker pull "${ecr_web_url}:${image_tag}"

# ---- docker-compose.yml -------------------------------------------------------
# Non-secret env values are baked in directly (they're not credentials, and
# they never change without a re-apply); secret values load from .env at
# container start via env_file, never interpolated into this file itself.
cat > /opt/margince/docker-compose.yml <<'COMPOSE_EOF'
services:
  api:
    image: "${ecr_api_url}:${image_tag}"
    restart: unless-stopped
    env_file: /opt/margince/.env
    environment:
      MARGINCE_CONFIG: /app/config/margince.yaml
      MARGINCE_REDIS: "${redis_host}:6379"
      MARGINCE_REDIS_TLS: "true"
      MARGINCE_PUBLIC_BASE_URL: "${public_base_url}"
      MARGINCE_BLOBSTORE_ENDPOINT: "s3.${aws_region}.amazonaws.com"
      MARGINCE_BLOBSTORE_BUCKET: "${blobstore_bucket}"
      MARGINCE_BLOBSTORE_REGION: "${aws_region}"
      MARGINCE_BLOBSTORE_USE_SSL: "true"
      MARGINCE_LOG_FORMAT: json
    volumes:
      - /opt/margince/config:/app/config:ro
    logging:
      driver: awslogs
      options:
        awslogs-region: "${aws_region}"
        awslogs-group: "${api_log_group}"
        awslogs-stream: api

  worker:
    image: "${ecr_worker_url}:${image_tag}"
    restart: unless-stopped
    env_file: /opt/margince/.env
    environment:
      MARGINCE_CONFIG: /app/config/margince.yaml
      MARGINCE_REDIS: "${redis_host}:6379"
      MARGINCE_REDIS_TLS: "true"
      MARGINCE_PUBLIC_BASE_URL: "${public_base_url}"
      MARGINCE_BLOBSTORE_ENDPOINT: "s3.${aws_region}.amazonaws.com"
      MARGINCE_BLOBSTORE_BUCKET: "${blobstore_bucket}"
      MARGINCE_BLOBSTORE_REGION: "${aws_region}"
      MARGINCE_BLOBSTORE_USE_SSL: "true"
      MARGINCE_LOG_FORMAT: json
      MARGINCE_OBSERVE_ADDR: "0.0.0.0:9101"
    volumes:
      - /opt/margince/config:/app/config:ro
    logging:
      driver: awslogs
      options:
        awslogs-region: "${aws_region}"
        awslogs-group: "${worker_log_group}"
        awslogs-stream: worker

  web:
    image: "${ecr_web_url}:${image_tag}"
    restart: unless-stopped
    logging:
      driver: awslogs
      options:
        awslogs-region: "${aws_region}"
        awslogs-group: "${web_log_group}"
        awslogs-stream: web

  nginx:
    image: nginx:1.27-alpine
    restart: unless-stopped
    depends_on: [api, web]
    ports:
      - "80:80"
%{ if enable_tls ~}
      - "443:443"
%{ endif ~}
    volumes:
      - /opt/margince/nginx.conf:/etc/nginx/conf.d/default.conf:ro
%{ if enable_tls ~}
      - /opt/margince/certbot/www:/var/www/certbot:ro
      - /opt/margince/certbot/conf:/etc/letsencrypt:ro
%{ endif ~}
%{ if enable_tls ~}

  # Never `up -d`'d on its own (no restart policy, entrypoint exits
  # immediately) — invoked directly below via `docker compose run` for
  # issuance, and by margince-renew.timer for renewal.
  certbot:
    image: certbot/certbot:latest
    entrypoint: "/bin/true"
    volumes:
      - /opt/margince/certbot/www:/var/www/certbot
      - /opt/margince/certbot/conf:/etc/letsencrypt
%{ endif ~}
COMPOSE_EOF

# ---- systemd unit: starts the stack on this boot and every one after -------
cat > /etc/systemd/system/margince.service <<'UNIT'
[Unit]
Description=Margince docker compose stack
After=docker.service network-online.target
Requires=docker.service
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/margince
ExecStart=/usr/bin/docker compose -f /opt/margince/docker-compose.yml up -d --remove-orphans
ExecStop=/usr/bin/docker compose -f /opt/margince/docker-compose.yml down
TimeoutStartSec=300

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now margince.service

%{ if enable_tls ~}
# ---- Real certificate: replaces the dummy one, then reloads nginx ----------
# Requires ${domain}'s DNS to already point at this instance's Elastic IP
# (README step 2) — the HTTP-01 challenge below is served over the internet,
# on the :80 nginx already opened as part of `margince.service` above.
docker compose -f /opt/margince/docker-compose.yml run --rm certbot certonly \
  --webroot -w /var/www/certbot \
  --email "${letsencrypt_email}" -d "${domain}" \
  --agree-tos --no-eff-email --non-interactive
docker compose -f /opt/margince/docker-compose.yml exec nginx nginx -s reload

# ---- Renewal: twice daily, certbot's own recommended cadence ---------------
# `certbot renew` on its own (no --webroot/-d needed) reuses the webroot path
# and domain recorded in /etc/letsencrypt/renewal/${domain}.conf from the
# issuance above, and no-ops for a cert that isn't yet due.
cat > /etc/systemd/system/margince-renew.service <<'RENEW_UNIT'
[Unit]
Description=Renew the margince nginx TLS certificate
After=margince.service
Requires=margince.service

[Service]
Type=oneshot
WorkingDirectory=/opt/margince
ExecStart=/usr/bin/docker compose -f /opt/margince/docker-compose.yml run --rm certbot renew --quiet
ExecStartPost=/usr/bin/docker compose -f /opt/margince/docker-compose.yml exec nginx nginx -s reload
RENEW_UNIT

cat > /etc/systemd/system/margince-renew.timer <<'RENEW_TIMER'
[Unit]
Description=Twice-daily margince TLS renewal check (certbot's own recommended cadence)

[Timer]
OnCalendar=*-*-* 00,12:00:00
RandomizedDelaySec=1h
Persistent=true

[Install]
WantedBy=timers.target
RENEW_TIMER

systemctl daemon-reload
systemctl enable --now margince-renew.timer
%{ endif ~}
