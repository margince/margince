# The image the merge gate's toolchain jobs run in: Go, the gate binaries,
# Node with pnpm, and Chromium with the libraries Playwright needs.
#
# Every byte comes from a pin in this repository, and only a pull request moves
# one. Base images are pinned by digest. Ubuntu packages come from one
# snapshot.ubuntu.com timestamp, each at an exact version. The gate binaries are
# what `make -n tools` prints, built with Go's checksum database on. pnpm is the
# root package.json's `packageManager`, hash and all. The Chromium download is
# checked against the sha256 below. backend/gates/ciimagepins_test.go holds
# each of these, and docs/explanation/ci-image.md says how the image is built
# and found.
#
# scripts/ci-image-context.sh writes the build context (this file, tools.sh,
# package.json) and names the image after a hash of it, so the image changes
# exactly when one of those does.

FROM golang:1.27.3-trixie@sha256:2336eda24930976591abea806d9b7d99eeae38238f19755c5deec02facdc1594 AS go

FROM node:24.22.2-trixie-slim@sha256:0820eaa85e07d0490446dd01d14707e17e6108a36988cf374899cd057eb3812d AS node

# The snapshot is one point in the archive's history, so the same timestamp
# and pins resolve to the same packages on every build. The image's own
# sources name the live archive, so they are replaced, not added to.
#
# snapshot.ubuntu.com answers only over HTTPS, and the base image has no CA
# bundle until ca-certificates is installed, so apt starts on the one the
# pinned Go image carries. BuildKit keys that COPY on the file's bytes, so a Go
# bump leaves this layer cached. apt checks every index against the Ubuntu
# archive key either way.
#
# Three tries with a 30 s timeout: a dead mirror fails the build in minutes.
# `--error-on=any` fails the update on an index that never came; without it apt
# only warns, and every pin after it reads as not found.
#
# The packages are the tools the gate jobs call, then the list
# `playwright install-deps chromium` installs on ubuntu24.04.
FROM ubuntu:24.04@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852 AS ubuntu
ARG UBUNTU_SNAPSHOT=20261105T143010Z
COPY --from=go /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN rm /etc/apt/sources.list.d/ubuntu.sources \
 && printf '%s\n' \
      'Types: deb' \
      "URIs: https://snapshot.ubuntu.com/ubuntu/${UBUNTU_SNAPSHOT}" \
      'Suites: noble noble-updates noble-security' \
      'Components: main universe' \
      'Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg' \
      > /etc/apt/sources.list.d/snapshot.sources \
 && printf '%s\n' \
      'Acquire::Retries "3";' \
      'Acquire::http::Timeout "30";' \
      'Acquire::https::Timeout "30";' \
      > /etc/apt/apt.conf.d/80snapshot \
 && apt-get update --error-on=any \
 && DEBIAN_FRONTEND=noninteractive apt-get upgrade -y \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates=20260816ubuntu0.24.04.1 \
      curl=8.5.0-2ubuntu10.16 \
      git=1:2.43.0-1ubuntu7.4 \
      jq=1.7.1-3ubuntu0.24.04.2 \
      make=4.3-4.1build2 \
      perl=5.38.2-3.2ubuntu0.6 \
      procps=2:4.0.4-4ubuntu3.3 \
      python3=3.12.3-0ubuntu2.1 \
      shellcheck=0.9.0-1 \
      tzdata=2026e-0ubuntu0.24.04.1 \
      unzip=6.0-28ubuntu4.1 \
      xz-utils=5.6.1+really5.4.5-1ubuntu0.4 \
      zstd=1.5.5+dfsg2-2build1.1 \
      fonts-freefont-ttf=20211204+svn4273-2 \
      fonts-ipafont-gothic=00303-21ubuntu1 \
      fonts-liberation=1:2.1.5-3 \
      fonts-noto-color-emoji=2.047-0ubuntu0.24.04.1 \
      fonts-tlwg-loma-otf=1:0.7.3-1 \
      fonts-unifont=1:15.1.01-1build1 \
      fonts-wqy-zenhei=0.9.45-8 \
      libasound2t64=1.2.11-1ubuntu0.3 \
      libatk-bridge2.0-0t64=2.52.0-1build1 \
      libatk1.0-0t64=2.52.0-1build1 \
      libatspi2.0-0t64=2.52.0-1build1 \
      libcairo2=1.18.0-3build1 \
      libcups2t64=2.4.7-1.2ubuntu7.14 \
      libdbus-1-3=1.14.10-4ubuntu4.1 \
      libdrm2=2.4.125-1ubuntu0.1~24.04.2 \
      libfontconfig1=2.15.0-1.1ubuntu2 \
      libfreetype6=2.13.2+dfsg-1ubuntu0.2 \
      libgbm1=25.2.8-0ubuntu0.24.04.4 \
      libglib2.0-0t64=2.80.0-6ubuntu3.10 \
      libnspr4=2:4.35-1.1build1 \
      libnss3=2:3.98-1ubuntu0.3 \
      libpango-1.0-0=1.52.1+ds-1build1 \
      libx11-6=2:1.8.7-1ubuntu0.1 \
      libxcb1=1.15-1ubuntu2 \
      libxcomposite1=1:0.4.5-1build3 \
      libxdamage1=1:1.1.6-1build1 \
      libxext6=2:1.3.4-1build2 \
      libxfixes3=1:6.0.0-2build1 \
      libxkbcommon0=1.6.0-1build1 \
      libxrandr2=2:1.5.2-2build1 \
      xfonts-cyrillic=1:1.0.5+nmu1 \
      xfonts-scalable=1:1.0.3-1.3 \
      xvfb=2:21.1.12-1ubuntu1.9 \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

# tools.sh is the recipe `make -n tools` prints, so the versions stay in
# backend/Makefile. The checksum database is on, as Go sets it by default.
# CGO is off so the binaries carry no link to the Go image's newer glibc.
# GOPROXY falls through to the source on any proxy error, as in the workflows.
FROM go AS gate-binaries
ENV GOPATH=/go GOTOOLCHAIN=local CGO_ENABLED=0 GOPROXY="https://proxy.golang.org|direct"
COPY tools.sh /tmp/tools.sh
RUN bash -e /tmp/tools.sh && rm /tmp/tools.sh

# The browser build the locked Playwright launches headless. Playwright
# publishes no checksum for it, so the sha256 is recorded here, and
# ciimagepins_test.go fails when the lockfile moves Playwright off this version.
FROM ubuntu AS chromium
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

FROM ubuntu
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
# root, so git would refuse the repository as owned by someone else. The checks
# after it prove each copied toolchain runs on this base.
COPY package.json /opt/pnpm-pin/package.json
RUN git config --system --add safe.directory '*' \
 && ln -s ../lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack \
 && corepack enable --install-directory /usr/local/bin pnpm \
 && cd /opt/pnpm-pin && COREPACK_ENABLE_NETWORK=1 corepack install \
 && pnpm --version \
 && go version \
 && node --version \
 && golangci-lint version \
 && shell="$(echo /ms-playwright/chromium_headless_shell-*/chrome-headless-shell-linux64/chrome-headless-shell)" \
 && test -x "$shell" \
 && libraries="$(ldd "$shell")" \
 && if echo "$libraries" | grep 'not found'; then echo "Chromium is missing the libraries above" >&2; exit 1; fi

ENV COREPACK_ENABLE_NETWORK=0
