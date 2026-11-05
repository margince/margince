# The image the merge gate's toolchain jobs run in: Go, the gate binaries,
# Node with pnpm, and Chromium with the libraries Playwright needs.
#
# Every byte comes from a pin in this repository, and only a pull request moves
# one. Base images are pinned by digest. Debian packages come from one
# snapshot.debian.org timestamp, each at an exact version. The gate binaries are
# what `make -n tools` prints, built with Go's checksum database on. pnpm is the
# root package.json's `packageManager`, hash and all. The Chromium download is
# checked against the sha256 below. backend/gates/ciimagepins_test.go holds
# each of these, and docs/explanation/ci-pipeline.md says how the image is built
# and found.
#
# scripts/ci-image-context.sh writes the build context (this file, tools.sh,
# package.json) and names the image after a hash of it, so the image changes
# exactly when one of those does.

FROM golang:1.27.3-trixie@sha256:2336eda24930976591abea806d9b7d99eeae38238f19755c5deec02facdc1594 AS go

FROM node:24.22.2-trixie-slim@sha256:0820eaa85e07d0490446dd01d14707e17e6108a36988cf374899cd057eb3812d AS node

# The snapshot is one point in the archive's history, so the same timestamp
# and pins resolve to the same packages on every build. Archived Release files
# expire by design, and a snapshot never changes, hence Check-Valid-Until off.
# snapshot-cloudflare.debian.org serves the same archive as snapshot.debian.org
# through a CDN, and apt checks every file against Debian's signed Release
# files either way; the origin host times out under load.
# proposed-updates is listed at priority 100: Renovate's package datasource
# lists every version the archive holds, a stable update waits there until the
# point release, and a pin may name one. Nothing else is taken from it.
FROM debian:13-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 AS debian
ARG DEBIAN_SNAPSHOT=20261105T143010Z
RUN printf '%s\n' \
      'Types: deb' \
      "URIs: http://snapshot-cloudflare.debian.org/archive/debian/${DEBIAN_SNAPSHOT}" \
      'Suites: trixie trixie-updates trixie-proposed-updates' \
      'Components: main' \
      'Signed-By: /usr/share/keyrings/debian-archive-keyring.pgp' \
      '' \
      'Types: deb' \
      "URIs: http://snapshot-cloudflare.debian.org/archive/debian-security/${DEBIAN_SNAPSHOT}" \
      'Suites: trixie-security' \
      'Components: main' \
      'Signed-By: /usr/share/keyrings/debian-archive-keyring.pgp' \
      > /etc/apt/sources.list.d/debian.sources \
 && printf '%s\n' \
      'Package: *' \
      'Pin: release n=trixie-proposed-updates' \
      'Pin-Priority: 100' \
      > /etc/apt/preferences.d/proposed-updates \
 && printf '%s\n' \
      'Acquire::Check-Valid-Until "false";' \
      'Acquire::Retries "5";' \
      > /etc/apt/apt.conf.d/80snapshot \
 && apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get upgrade -y \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates=20250419+deb13u1 \
      curl=8.14.1-2+deb13u7 \
      git=1:2.47.3-0+deb13u1 \
      jq=1.7.1-6+deb13u4 \
      make=4.4.1-2 \
      perl=5.40.1-6+deb13u1 \
      procps=2:4.0.4-9 \
      python3=3.13.5-1 \
      shellcheck=0.10.0-1 \
      tzdata=2026f-0+deb13u1 \
      unzip=6.0-29+deb13u1 \
      xz-utils=5.8.1-1+deb13u2 \
      zstd=1.5.7+dfsg-1 \
      fonts-freefont-ttf=20211204+svn4273-2 \
      fonts-ipafont-gothic=00303-23 \
      fonts-liberation=1:2.1.5-3 \
      fonts-noto-color-emoji=2.051-0+deb13u1 \
      fonts-tlwg-loma-otf=1:0.7.3-1 \
      fonts-unifont=1:15.1.01-1 \
      fonts-wqy-zenhei=0.9.45-8 \
      libasound2t64=1.2.14-1+deb13u1 \
      libatk-bridge2.0-0t64=2.56.2-1+deb13u2 \
      libatk1.0-0t64=2.56.2-1+deb13u2 \
      libatspi2.0-0t64=2.56.2-1+deb13u2 \
      libcairo2=1.18.4-1+b1 \
      libcups2t64=2.4.10-3+deb13u2 \
      libdbus-1-3=1.16.2-2 \
      libdrm2=2.4.124-2 \
      libfontconfig1=2.15.0-2.3 \
      libfreetype6=2.13.3+dfsg-1+deb13u1 \
      libgbm1=25.0.7-2+deb13u1 \
      libglib2.0-0t64=2.84.4-3~deb13u5 \
      libnspr4=2:4.36-1 \
      libnss3=2:3.110-1+deb13u5 \
      libpango-1.0-0=1.56.3-1 \
      libx11-6=2:1.8.12-1 \
      libxcb1=1.17.0-2+b1 \
      libxcomposite1=1:0.4.6-1 \
      libxdamage1=1:1.1.6-1+b2 \
      libxext6=2:1.3.4-1+b3 \
      libxfixes3=1:6.0.0-2+b4 \
      libxkbcommon0=1.7.0-2 \
      libxrandr2=2:1.5.4-1+b3 \
      xfonts-scalable=1:1.0.3-1.3 \
      xvfb=2:21.1.16-1.3+deb13u4 \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

# tools.sh is the recipe `make -n tools` prints, so the versions stay in
# backend/Makefile. The checksum database is on, as Go sets it by default.
FROM go AS gate-binaries
ENV GOPATH=/go GOTOOLCHAIN=local
COPY tools.sh /tmp/tools.sh
RUN bash -e /tmp/tools.sh && rm /tmp/tools.sh

# The browser build the locked Playwright launches headless. Playwright
# publishes no checksum for it, so the sha256 is recorded here, and
# ciimagepins_test.go fails when the lockfile moves Playwright off this version.
FROM debian AS chromium
ARG PLAYWRIGHT_VERSION=1.63.0
ARG CHROMIUM_REVISION=1243
ARG CHROMIUM_VERSION=153.0.8010.12
ARG CHROMIUM_SHA256=a9da028861a0cf789ff25c2fed45f5f1aaf969ed9247835b6a7821a4f7af9d1d
RUN dir="/ms-playwright/chromium_headless_shell-${CHROMIUM_REVISION}" \
 && mkdir -p "$dir" \
 && curl -fsSL --retry 5 -o /tmp/shell.zip \
      "https://cdn.playwright.dev/builds/cft/${CHROMIUM_VERSION}/linux64/chrome-headless-shell-linux64.zip" \
 && echo "${CHROMIUM_SHA256}  /tmp/shell.zip" | sha256sum -c - \
 && unzip -q /tmp/shell.zip -d "$dir" \
 && rm /tmp/shell.zip \
 && touch "$dir/INSTALLATION_COMPLETE"

FROM debian
COPY --from=go /usr/local/go /usr/local/go
COPY --from=gate-binaries /go/bin /go/bin
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules/corepack /usr/local/lib/node_modules/corepack
COPY --from=chromium /ms-playwright /ms-playwright

# GOTOOLCHAIN=local: a go.mod past this Go fails loudly, never downloads.
# COREPACK_ENABLE_NETWORK=0: pnpm is the one baked below, never a fetch.
# MARGINCE_CI_IMAGE tells the cache actions they restore into this image,
# whose paths differ from a bare runner's.
ENV GOPATH=/go \
    GOTOOLCHAIN=local \
    PATH=/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    COREPACK_HOME=/opt/corepack \
    COREPACK_ENABLE_DOWNLOAD_PROMPT=0 \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    MARGINCE_CI_IMAGE=1

# A job's checkout is owned by the runner's user, and every step here runs as
# root, so git would refuse the repository as owned by someone else.
COPY package.json /opt/pnpm-pin/package.json
RUN git config --system --add safe.directory '*' \
 && ln -s ../lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack \
 && corepack enable --install-directory /usr/local/bin pnpm \
 && cd /opt/pnpm-pin && COREPACK_ENABLE_NETWORK=1 corepack install \
 && pnpm --version \
 && go version \
 && node --version \
 && missing="$(ldd /ms-playwright/chromium_headless_shell-*/chrome-headless-shell-linux64/chrome-headless-shell | grep 'not found' || true)" \
 && { [ -z "$missing" ] || { echo "Chromium is missing libraries: $missing" >&2; exit 1; }; }

ENV COREPACK_ENABLE_NETWORK=0
