<!-- describes: scripts/lib-devready.sh scripts/dev.sh -->
# Set up development on Windows with WSL

Margince builds and runs on Linux and macOS. On Windows, run it inside WSL 2 (Windows Subsystem for
Linux) and treat WSL as the Linux machine: the checkout, the tools and every `make` command live there.

## Pick the path you need

To use Margince, you need no toolchain. Get a release and start its container images with Docker
Compose, as [run-with-docker-compose.md](run-with-docker-compose.md) says. Docker Desktop on Windows
runs those images without WSL setup of your own.

To change the code, you need the developer toolchain inside WSL: Go 1.27 or later, Docker with its
Compose plugin, `jq`, `golangci-lint`, Node 24 and pnpm. The rest of this page is for that path. The
steps after setup are the same as on Linux, in the [README](../../README.md#run-it-from-the-source).

## Put the checkout in the Linux file system

Clone into your WSL home directory, for example `~/src/margince`, and not under `/mnt/c/`.

A path under `/mnt/c/` is a Windows drive. WSL reaches each file there through a bridge to Windows, so a
Go build, a pnpm install and Vite's file watching are slow, and Vite can miss an edit. `make dev` prints a
warning when it starts from such a checkout under WSL.

To edit the files from Windows, open them through `\\wsl$\<distro>\home\<you>\src\margince`, or use an
editor that connects to WSL. Do not move the checkout to the Windows drive for that.

## Keep the WSL session open

`make dev` starts the API, the worker and Vite in the background and returns. They belong to the WSL
session that started them. When the last WSL terminal closes, WSL can stop the distribution after a short
idle time, and the stack then goes with it. The idle settings are in Microsoft's
[WSL configuration docs](https://learn.microsoft.com/windows/wsl/wsl-config).

Start the stack from a WSL terminal and keep that terminal open while you use the app. A one-off
`wsl make dev` from PowerShell returns at once, and the stack can stop soon after. To check from Windows
that the distribution still runs:

```powershell
wsl --list --running
```

Stop the stack with `make dev-stop` before you close the terminal, so the next `make dev` finds its ports
free.

## Use one Docker

`make dev` runs Postgres, Redis and MinIO as containers, and checks Docker before it starts. If Docker, its
Compose plugin or its daemon is missing, it stops and names the missing part.

There are two common ways to get Docker inside WSL:

- Docker Desktop on Windows, with WSL integration turned on for your distribution (Settings, Resources,
  WSL integration).
- Docker Engine and the `docker-compose-plugin` package installed inside the distribution.

Use one of them. With both, the `docker` command can talk to a different daemon from the one you expect.
Containers then seem to vanish, and a port such as 15432 is held by a container the current daemon does
not list.

### Check which Docker you reach

These commands only read. None of them deletes or changes anything:

```sh
which -a docker
docker context ls
docker context show
echo "${DOCKER_HOST:-not set}"
ls -l /var/run/docker.sock
docker version
docker compose version
docker info --format '{{.Name}} {{.OperatingSystem}} {{.ServerVersion}}'
```

How to read them:

- `which -a docker` lists every `docker` program on your `PATH`. More than one means two installs.
- `docker context ls` marks the selected context with `*`. `DOCKER_HOST`, when set, wins over it.
- `docker info` names the daemon that answers. `Docker Desktop` in its operating system means Docker
  Desktop; a Linux name such as `Ubuntu` means an engine inside the distribution.
- `docker compose version` fails when the Compose plugin is missing. The old standalone `docker-compose`
  does not count, because the Makefile runs `docker compose`.

To see where a port went, ask each context in turn. This lists containers and changes nothing:

```sh
docker --context <name> ps -a --format '{{.Names}} {{.Ports}} {{.Status}}'
```

Once you know which install you keep, stop the other one's service in its own settings. Remove an install
or its containers only when you know they hold no data you need. `make dev` never does either for you.

## Start the stack

From the checkout in WSL:

```sh
make install
make dev
make seed-dev
make verify-boot
```

Open http://localhost:8080 in a Windows browser. WSL forwards `localhost` ports to Windows. `make dev`
says the stack is ready only when the app port also reaches the API, which is the route `make seed-dev`
and `make verify-boot` use. If one of them still cannot reach the stack, its message names which part
does not answer: the API itself or the web app's dev server in front of it, with the log to read.
