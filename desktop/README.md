<!-- prose:plain -->
# desktop: the self-contained macOS and Windows builds

One folder that runs the whole Margince stack on your machine, with no Docker and nothing to install
first. The stack is Postgres, the event bus, the API, the worker and the web app. The user starts it, and
Margince opens in their browser.

**Status: proof of concept.** It starts, migrates, serves the app and lives through a restart. It is
not signed for others to install, and many surfaces are off by default.

```sh
make desktop        # macOS   -> build/desktop/margince/          (first run ~5 min)
make desktop-win    # Windows -> build/desktop/margince-windows/  (must run ON Windows)
```

Each platform builds on itself. The macOS lane compiles Postgres and Valkey with the Xcode tools,
and the Windows lane needs MSVC for pgvector and MSYS2 for Redis. Neither one builds for the other.

The macOS folder most often cannot run from `build/desktop/`. The socket path may be at most 103
bytes. A repository checkout is most often deep enough to pass that, depending on where you cloned
it. The launcher measures the path and says so. Copy the folder to a shorter path, then start
it:

```sh
cp -R build/desktop/margince ~/Margince
cd ~/Margince && ./margince
```

Windows has no such limit. Postgres listens on loopback TCP, so the folder runs from any place.

## The documentation

This folder holds only the sources. All else is in `docs/`, so there is one copy to keep true:

- **[How to build, run, configure and update it](../docs/how-to/build-the-desktop-app.md)**: every
  `make desktop-*` target on both platforms, the settings file, how to update, and the failure table.
- **[Why it has this shape](../docs/explanation/desktop-distribution.md)**: why it must carry its own
  Postgres, since pgvector is not in `contrib`. It covers how a gate makes sure the folder can move. It also
  covers the update contract the layout holds, why the two platforms differ where they do, and the
  known limits.
- **[Every flag and environment variable](../docs/reference/configuration.md)**

## What is here

| Path | What it is |
|---|---|
| `build/macos-target.sh` | macOS: the one place the supported OS floor lives; pins `MACOSX_DEPLOYMENT_TARGET` and fails the build if any binary needs newer |
| `build/build-postgres.sh` | macOS: relocatable Postgres 16 + pgvector + contrib, compiled from source |
| `build/build-valkey.sh` | macOS: the event bus |
| `build/build-app.sh` | macOS: api, worker, migrate, frontend, launcher |
| `build/build-dist.sh` | macOS: assemble and verify the distributable folder |
| `build/common.ps1` | Windows: shared helpers: pinned downloads, the MSVC shell, fail-on-native-error |
| `build/build-postgres.ps1` | Windows: stage PostgreSQL 16, compile pgvector against it with MSVC |
| `build/build-bus.ps1` | Windows: the event bus, Redis 7.2 built under MSYS2 |
| `build/build-app.ps1` | Windows: api, worker, migrate, frontend, launcher |
| `build/build-dist.ps1` | Windows: assemble and verify the distributable folder |
| `build/build-windows.ps1` | Windows: all four in order; the entry point, since a Windows host need not have `make` |
| `../.github/workflows/desktop-macos.yml` | The macOS lane on a runner: the only thing that proves these scripts still work, since they cannot run elsewhere |
| `../.github/workflows/desktop-windows.yml` | The same for Windows |
| `launcher/` | The Go supervisor: stdlib only and outside `go.work`, so it imports no product code. It starts the shipped binaries as child processes |

### The launcher is one program, split where the platforms differ

`main.go`, `layout.go`, `envfile.go`, `web.go` and `services.go` are shared. The folder layout, the
settings file, the SPA server and the proxy list are the same product on both. Only the files Go picks
by name differ, and each one says in its header why:

| File | What is different, and why it has to be |
|---|---|
| `postgres_unix.go` / `postgres_windows.go` | macOS: unix socket, trust auth, supervised child, SIGINT. Windows: no socket exists, so loopback + `scram-sha-256`, and `pg_ctl` because `postgres.exe` refuses to run under an administrator |
| `process_unix.go` / `process_windows.go` | POSIX signals vs. a `CTRL_BREAK` console event to the child's own process group |
| `platform_unix.go` / `platform_windows.go` | Executable suffix, which bus binary ships, how the local timezone is read, how a browser is opened, and whether the console has to be held open to read a failure |
