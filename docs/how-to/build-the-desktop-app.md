<!-- prose:plain -->
# Build the desktop app

Build the folder that holds everything, for a user who does not write code, on macOS or Windows. It needs no
Docker and no services to set up. Postgres, the event bus, the API, the worker and the web app start from
one launcher, and the user works in a browser.

Why it has this shape is in [explanation/desktop-distribution.md](../explanation/desktop-distribution.md).
That page covers the custom Postgres, the update contract, why the two systems are different where they are,
and the limits.

## Will it run on this machine?

What a **user** needs. Building it needs more; see the next section.

| | macOS bundle | Windows bundle |
|---|---|---|
| **OS** | macOS 13 Ventura or newer | Windows 10 or newer. Server 2016+ shares that system core and should work, but **is not tested**: no one has started the bundle there |
| **Chip** | The same as the build machine; the bundle does **not** run on every chip. A build on Apple silicon does not run on an Intel Mac at all. An Intel build runs on Apple silicon under Rosetta 2. `make desktop-dist` prints which one it made | `x64` only. Windows on ARM can run `x64` code, but no ARM build is made and none is tested |
| **Must already be installed** | Nothing | The Microsoft Visual C++ files in [vc_redist.x64.exe](https://aka.ms/vs/17/release/vc_redist.x64.exe). Most machines have it, but it is **not** in the bundle |
| **Admin rights** | Not needed | Not needed. Running as an admin also works: `pg_ctl` drops the rights that Postgres refuses to start with |
| **Warning on first start** | Signed by the build machine only. So a copy from a browser download is marked, and Gatekeeper refuses it **once**. Right-click → **Open**, and the bundle clears the rest itself (see below). A copy made with `cp`, USB or AirDrop is not marked and needs none of this | Not signed, so SmartScreen blocks it: **More info** → **Run anyway** |
| **Where it is put** | The path must be short. The database socket path has a system limit of 103 bytes, and the launcher measures it and says so | Any folder. There is no socket, so no limit |
| **Browser** | Chrome or Edge 111+, Firefox 114+, or Safari 16.4+ (Safari 16.4 runs on macOS 11 and later, so every supported version can reach it) | Chrome or Edge 111+, or Firefox 114+ |

The oldest OS versions are held by the build. `MACOSX_DEPLOYMENT_TARGET` is pinned to 13.0, and **the build
fails** if any shipped binary needs a newer one. So the bundle cannot take on the macOS version of the build
machine. On Windows the oldest version is the one PostgreSQL 16 needs (Windows 10 or newer). Go and
`MSYS2` need the same.

Both need about 1 GB free for the folder and the database. Neither writes a single byte outside its own
folder: no installer, no registry keys, no `~/Library`, no `%APPDATA%`. Attachments are the one case outside it,
if you move them to another place yourself (see below).

## Or download one already built

There are two places, for two different needs.

### A release, for a build you keep

The [releases page](https://github.com/margince/margince/releases) carries both bundles as files on every
release that was made with them (`margince-macos-<version>.tar.gz` and `margince-windows-<version>.zip`). The
release notes hold the first-start steps. Release files are kept for good: pick a version, and download its build.

To make one, start the **Release** workflow by hand from the Actions tab. A normal merge to main does not
build desktop bundles. Each bundle builds Postgres from source, and a merge asks no new question about it. A
build to download exists because someone decided it should.

### A run artifact, for testing a change

Each system also has a CI lane that publishes the folder as a run artifact. So testing a branch needs no build
tools at all. The lane also proves that it still works, since neither half can be built on the other system.
These files are removed after 14 days.

| Workflow | Runner | Artifact |
|---|---|---|
| `desktop-macos` | `macos-latest` (Apple silicon) | `margince-macos-<sha>`: a **tarball**, because the artifact does not keep the file mode that makes a file run |
| `desktop-windows` | `windows-latest` (`x64`) | `margince-windows-<sha>`: a plain folder |

Both start on their own when `desktop/**` changes on a pull request. They also run by hand from the Actions tab,
and as workflows that **Release** calls when it makes a build to download. It is the same lane each time, so a
release bundle cannot be different from the one a pull request was checked against. Download from the run page,
or:

```
gh run download <run-id> -n margince-macos-<sha>
tar -xzf margince-macos.tar.gz          # keeps the +x bit and the signatures
```

Neither build is signed for release, so the first start still needs the Gatekeeper or SmartScreen step below.

## What building it needs

**Each system builds on itself.** Neither half builds for the other. The macOS lane builds Postgres and Valkey
with the Xcode tools. On Windows, pgvector has no build system other than `nmake` against MSVC.

| | Build host | Also needs |
|---|---|---|
| macOS | Apple silicon or Intel; the bundle takes the chip of the build machine | Xcode Command Line Tools (`xcode-select --install`), Go, Node and pnpm |
| Windows | `x64` | [Visual Studio Build Tools](https://visualstudio.microsoft.com/downloads/) with the `Desktop development with C++` part (for pgvector), [msys2.org](https://www.msys2.org/) with `base-devel gcc` (for the event bus), Go, Node and pnpm |

## Build it on macOS

```
make desktop
```

The result is `build/desktop/margince/` (about 128 MB). The first run builds Postgres and pgvector from source
and takes about five minutes. Later runs skip that and finish in seconds, because Postgres changes only when
its pinned version does.

| Target | What it does |
|---|---|
| `make desktop` | The whole folder. Uses a Postgres and bus that are already built |
| `make desktop-rebuild` | Builds everything, including Postgres and the bus |
| `make desktop-postgres` | Only the Postgres 16 that can move between folders, with pgvector and `contrib` (about 5 minutes) |
| `make desktop-valkey` | Only the event bus |
| `make desktop-app` | Only `api`, `worker` and `migrate`, the frontend and the launcher |
| `make desktop-dist` | Only puts the folder together and checks the signatures |
| `make desktop-clean` | Removes all of `build/desktop/` |

Run `make desktop-postgres` again after you change the pinned Postgres or pgvector version in
`desktop/build/build-postgres.sh`. The checksums are pinned there. A wrong checksum fails the build; it does not
use a tarball from an earlier run.

## Build it on Windows

Run this **on Windows**. PowerShell is the way in, because a Windows build host does not need GNU make:

```powershell
powershell -ExecutionPolicy Bypass -File desktop\build\build-windows.ps1
```

The result is `build\desktop\margince-windows\`. The first run downloads PostgreSQL, a file of 310 MB, and
builds pgvector and Redis. Later runs use both again, and finish in the time the Go and frontend builds take.
Add `-Force` to build them again anyway.

If `make` and `pwsh` are there, the same lane has make targets that call the scripts:

| Target | Script | What it does |
|---|---|---|
| `make desktop-win` | `build-windows.ps1` | The whole folder. Uses a Postgres and bus that are already in place |
| `make desktop-win-rebuild` | `build-windows.ps1 -Force` | Builds everything |
| `make desktop-win-postgres` | `build-postgres.ps1` | Puts PostgreSQL 16 in place and builds pgvector against it (needs MSVC) |
| `make desktop-win-bus` | `build-bus.ps1` | The event bus: Redis 7.2 built under `MSYS2` |
| `make desktop-win-app` | `build-app.ps1` | `api`, `worker` and `migrate`, the frontend and the launcher |
| `make desktop-win-dist` | `build-dist.ps1` | Puts the folder together and checks that it runs alone |
| `make desktop-clean` | (none) | Removes all of `build/desktop/`, for both systems |

The pinned versions and checksums live at the top of each script. A wrong checksum fails the build; it does not
use a download from an earlier run.

## Run it

### macOS

**Copy the folder to a short path first.** The database uses a socket file inside the folder, and the path has a
system limit of 103 bytes. The repository's own `build/desktop/margince/` is in most cases past the limit (by how much
depends on where you put the repository). The launcher then refuses to start. It names the limit and the measured length,
and tells you to move the folder.

```
cp -R build/desktop/margince ~/Margince
cd ~/Margince && ./margince
```

Or click twice on `Start Margince.command` in Finder, which is what a user who does not write code does.

**A downloaded copy warns once.** The build is signed by the build machine only, because Developer ID signing
and notarization need an Apple account that costs money. So Gatekeeper refuses it: right-click → **Open** → **Open**, or
approve it in System Settings → Privacy & Security.

It warns once for the whole bundle. The browser marks the download, and the tool that opens the download copies that
mark to every file it takes out. Gatekeeper asks when a program is *run*.

Without help, a bundle would show a separate
dialog for `initdb`, `postgres`, `valkey-server`, `migrate`, `api` and `worker` as the stack comes up. Each one
would block the start. The starter clears the mark from the launcher, and the launcher clears it from
`runtime/` before it starts anything. That leaves the single dialog above. Neither touches `data/`.

A copy that never passed through a browser (`cp`, USB, AirDrop) has no mark and shows nothing. To clear a
folder you already downloaded, run:

```
xattr -dr com.apple.quarantine ~/Margince/runtime
```

### Windows

There is no socket, so there is no path limit; copy it to any folder:

```powershell
Copy-Item -Recurse build\desktop\margince-windows $HOME\Margince
& $HOME\Margince\margince.exe
```

Or click twice on `Start Margince.cmd`, which runs it in its own command prompt. **The first start shows a
SmartScreen warning**: the build is not signed, and Authenticode signing needs a certificate that costs money. "More info"
→ "Run anyway".

### On both systems

It prints the address, opens the browser, and runs until Ctrl-C. The first start makes the settings, sets up
the database and applies the whole migration history. So it takes some seconds longer than later starts.

It prints the sign-in email and, **on the first start only**, the generated password. Later starts point at
`data/admin-password`, which is the only copy.

## Set it up

Every feature you can add is off by default. Turn features on in `margince.env`, next to the launcher. The first
run writes that file with every supported setting written out and turned off in a comment. So it is also the
list of what exists. That is Gmail and Outlook capture, outbound webhooks, log level, the port, and the keys
that run the AI features.

Attachments and company logos are **not** in that list: they already work. The launcher keeps their bytes in
`data/blobs` inside the folder, with the database and the rest of the user's records. So an update leaves them
alone.

Set `MARGINCE_BLOBSTORE_PATH` to move them to another place. Or set `MARGINCE_BLOBSTORE_ENDPOINT` to keep
them in a service that works like `S3`. The endpoint comes first when both are set. No `S3` server comes in the
bundle, and none is needed.

**To back up**, copy `data/` and the place where the objects are. At the default they are inside `data/`, so a copy
of that folder is all the records of the installation. If you point `MARGINCE_BLOBSTORE_PATH` to another place,
a copy of `data/` is a database whose attachment rows name bytes it does not hold. `margince.yaml` and
`margince.env` are beside `data/` and are part of a restore too. They are not made again, and `margince.yaml`
decides which company this database is for.

A local store does not give what a service gives. It keeps no copies on other servers and no versions. It has
no signed links, and no sharing between machines of its own. Put the folder where two machines can see it, and
they see the same bytes. But nothing here sets that up or keeps them in step. For one user's installation that is enough; for
anything more, set the endpoint.

```
# margince.env
ANTHROPIC_API_KEY=sk-ant-...
MARGINCE_PORT=8801
```

Start it again to apply. A bad line stops the start and names the file and line; it is never skipped. Field
reference: [reference/configuration.md](../reference/configuration.md).

Company name, currency and time zone live in `margince.yaml`. Both files are made once and never written over,
so your edits stay when you start it again and when you update.

**On Windows, check the time zone.** Windows records its own zone name, not the IANA name this field takes. So a
Windows installation is made with `UTC`, and you must correct the value once.

For a real model, open **Settings → AI** in the running app. Bind each tier, and put the provider's key under
**Model provider keys**. You need both: a key for a provider that no tier uses changes nothing. A stored binding that
can serve comes before the stand-in model of the launcher. So it works without a new start, within the
time the routes take to read it, and there is no file to place. See
[connect-a-cloud-model-provider.md](connect-a-cloud-model-provider.md).

## Update an installation

Replace **the launcher, the starter script, and `runtime/`**. Leave `margince.yaml`, `margince.env` and
`data/` alone: they are the user's, and `data/` is the database.

**Quit it first**, so nothing holds the database while its binaries change. Then delete `runtime/` before you
copy the new one. A copy over the top leaves behind any file the new version dropped. An old library file beside
a new binary is a failure with no clear cause.

```
# macOS
rm -rf ~/Margince/runtime
cp -R build/desktop/margince/runtime ~/Margince/
cp build/desktop/margince/margince ~/Margince/
cp "build/desktop/margince/Start Margince.command" ~/Margince/
```

```powershell
# Windows
Remove-Item -Recurse -Force $HOME\Margince\runtime
Copy-Item -Recurse build\desktop\margince-windows\runtime $HOME\Margince\
Copy-Item -Force build\desktop\margince-windows\margince.exe $HOME\Margince\
Copy-Item -Force "build\desktop\margince-windows\Start Margince.cmd" $HOME\Margince\
```

An update replaces only those three things. If you replace the whole folder, you erase the records.

## Start over

```
rm -rf ~/Margince/data ~/Margince/margince.yaml ~/Margince/margince.env
```

```powershell
Remove-Item -Recurse -Force $HOME\Margince\data, $HOME\Margince\margince.yaml, $HOME\Margince\margince.env
```

The next start sets up a new installation with a new password. To remove everything, delete the folder. Nothing
is stored outside it, unless you moved the attachments with `MARGINCE_BLOBSTORE_*`: delete that place too.

## When something goes wrong

Logs are in `data/logs/`: `api.log`, `worker.log`, `postgres.log`, `bus.log`. The output of the launcher covers
start and stop only; each service writes its own file.

| What you see | Cause |
|---|---|
| `"the installation folder is too deeply nested"` | macOS only: the socket path is over 103 bytes. Move the folder to a shorter path in your home folder |
| `"address already in use"` | Another program holds the port, or a copy from before is still running. Quit it, or set `MARGINCE_PORT` |
| `"expected KEY=value"` | A bad line in `margince.env`, named with its line number |
| `"a database from a previous session is still running"` | Windows: a launcher was ended before it stopped Postgres, and the Postgres it left behind could not be stopped. Sign out and back in |
| Windows SmartScreen blocks the first start | The build is not signed. "More info" → "Run anyway" |
| Windows: `"VCRUNTIME140.dll was not found"` | The Microsoft Visual C++ files in [vc_redist.x64.exe](https://aka.ms/vs/17/release/vc_redist.x64.exe) are not installed, and the bundle does not include it |
| Attachments or logos fail | The launcher cannot write to `data/blobs`, or the place that `MARGINCE_BLOBSTORE_*` names cannot be reached. Check that setting, or remove it to use the default |
| AI answers look canned | No tier has a model, or the provider of that model has no key. So the stand-in model of the launcher runs the AI features. Open **Settings → AI**: bind each tier, and put the provider's key under **Model provider keys** |
| `"no licence is configured and this installation is production"` | `MARGINCE_ENV` was set to `production` in `margince.env` with no `MARGINCE_LICENSE` beside it. Give the token, or remove that line so the default `dev` mode applies |
| Dates and times look wrong on Windows | The first run set the time zone to `UTC`. Set `timezone` in `margince.yaml` |

To stop a copy that does not stop, find it by its port, not by its name. On macOS it starts as
`./margince`, so a `pkill -f` on the full path does not match it. **Use your own port** if `margince.env` sets
`MARGINCE_PORT`; 8800 is only the default.

```
kill -INT "$(lsof -nP -iTCP:8800 -sTCP:LISTEN -t)"
```

```powershell
Stop-Process -Id (Get-NetTCPConnection -LocalPort 8800 -State Listen).OwningProcess
```
