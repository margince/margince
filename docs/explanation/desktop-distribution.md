<!-- prose:plain -->
# Desktop build: one folder, no Docker

Margince normally runs as a server: Docker images, a hosted Postgres, and an
operator who sets it up. The desktop build is one folder that a user who has
never run a server gets, starts and uses in their browser. It needs no Docker,
no command-line setup and no services to set up. On Windows, one thing must
already be on the machine, and the how-to names it. That is the Microsoft
Visual C++ runtime for `x64`, which this build does not ship.

It exists for a single kind of user: one user, one machine, their own CRM.
That user is the reason it exists. Anyone who can run `docker compose up` gets
more from [deployment.md](../deployment.md). For them, this build would cost
more to maintain than it gives.

Both macOS on Apple silicon and Windows on `x64` are built. They are one
product and one launcher. Where they are different, the platform gave no
choice, and each such case is named below.

To build it, see [how-to/build-the-desktop-app.md](../how-to/build-the-desktop-app.md).

## Why it needs its own Postgres

Most of the cost to maintain it is building Postgres with pgvector on each
platform. Putting the folder together is a small cost next to that.

The database needs four extensions:

| Extension | Required by |
|---|---|
| `vector` | `backend/migrations/core/0001_baseline.up.sql` |
| `unaccent`, `pg_trgm` | `backend/migrations/core/0001_baseline.up.sql` |
| `btree_gist` | `backend/migrations/core/0001_baseline.up.sql` |

Three are `contrib` modules, which every ready-made Postgres build ships.
`vector` is not. pgvector is a third-party extension that must be compiled
against the Postgres build it loads into. No ready-made build can carry it, so
every platform here owns a compile step.

It is also required. `CREATE EXTENSION vector` runs in the first migration, so
a Postgres without it fails on the first start, rather than running without
`vector` search.

So the compile is work that never ends. Postgres ships fix releases about four
times a year, and pgvector releases on its own schedule. Each one means a new
build on both platforms.

### Free to move, and how that is enforced

The folder runs from any place the user put it, so nothing inside may point at
a full path outside itself. How much work that takes depends on the loader of
each platform, and the two platforms are far apart.

**macOS** writes a full install path into every Mach-O load command when a
binary is linked. `build-postgres.sh` changes them to `@rpath` and then signs
every changed file again. `install_name_tool` breaks a signature, and `arm64`
macOS refuses to run a binary whose signature is wrong. The build then checks
that no binary links to `/opt/homebrew`, `/usr/local`, or the stage folder it
was built in. The stage folder is the path that the move step removes, and the
one a check can miss.

**Windows** finds a DLL in the folder of the program that loads it. So a tree
copied out of a zip can already move, and there is nothing to change. The
Windows lane pins and checks the zip that the Postgres project ships, instead
of compiling Postgres itself. The platform already gives what the macOS compile
exists to produce. Only pgvector is compiled, with MSVC, against that staged
tree.

### The other kind of move: which OS the binary says it needs

A binary that finds every file it links to can still refuse to start. A Mach-O
carries the oldest macOS it will run on, and the loader enforces it. `clang`
sets that from the machine doing the build.

So a build with no pin marks the
binary with whatever OS the builder was running that day. It works on the
build machine and on anything newer. It fails on everything older, with a floor
nobody decided or wrote down. That floor goes up, silently, the next time the
builder takes an OS update.

Measured on a Mac running 15.7, a C file compiled with no target set reports
`minos 15.0`. A Go binary from the same tree reports `minos 13.0`, because Go
sets its own. Without a pin, the floor of the bundle is the newest of its
parts, and the two parts of one folder disagree by two versions.

`desktop/build/macos-target.sh` is the one place that number lives. It sets
`MACOSX_DEPLOYMENT_TARGET=13.0`, which is the floor of Go itself for this Go
version, so the C and Go parts agree. `assert_min_os` reads every shipped
binary again and fails the build if any of them needs a newer OS. The check
exists because a setting that is not set falls back silently to the OS version
of the build machine. The build machine is the one place where that failure
never shows.

Architecture is the limit that stays. The bundle matches the builder: Apple
silicon or Intel, never both in one binary. A Postgres for both would mean
building it twice and merging the results with `lipo`, and the user is one
user on one machine. `build-dist.sh` shows the architecture, so nobody has to
find it out.

The two lanes check the same claim in the way each platform can. macOS reads
the link table. Windows runs each third-party binary out of the built folder.
A missing DLL cannot show on the build machine, where the file is on `PATH` in
any case. On the machine of the user, it stops the app. So the check runs
where the copy of the user will be, instead of where the compiler was.

## The folder, and the update contract

```
margince/
├── margince / margince.exe        ← replaced by an update
├── Start Margince.command / .cmd  ← replaced by an update
├── runtime/                       ← replaced by an update
│   └── pgsql/  the bus  api  worker  migrate  web/
├── margince.yaml                  ← the user's: company name, currency, timezone
├── margince.env                   ← the user's: every optional feature
└── data/                          ← the user's: database, logs, uploads
```

Every path starts from this folder. Nothing is written to `~/Library` or
`%APPDATA%`, and nothing goes to any folder outside it. So the folder can be
moved, copied to another machine with the same OS and CPU architecture, or
deleted as one unit.

The split keeps the data of the user safe. An update writes over the launcher,
the starter and `runtime/`, and nothing else. A user who has never run a server
updates by copying new files over an existing folder. If durable data lived
under the part an update writes over, that copy would erase all the records.
The folder shape makes the obvious move safe, without the user having to follow
steps.

### Why the program folder is `runtime/` and not `resources/`

`codesign` reads a folder that holds both a program file of the same name and a
folder called `resources` as an old kind of bundle. It then tries to sign the
whole folder, walks into it, and fails on the `.command` starter as a part it
cannot sign. `codesign --verify` on the launcher then reports that the code
`has no resources but signature indicates they must be present`.

A new folder name removes the problem. For the same reason, binaries are signed
in the stage folder, where no path can be read as a bundle. A signature lives
inside the Mach-O and stays through the copy into the folder. So the step that
builds the folder checks the signatures and does not sign.

Windows has no such rule. The name is kept all the same, because one folder
shape means one document, one way to update and one `layout.go`.

## How it runs

The launcher is a supervisor, and it is not a second composition root. It
starts the shipped binaries as child processes and imports none of them. It is
a Go module that imports nothing outside Go itself, kept outside `go.work`. So
it neither sees nor changes the dependencies of the backend.

1. Reads `margince.env`; writes `margince.yaml` on first run.
2. Sets up `data/pg` if it is missing, then starts Postgres.
   On macOS it uses a unix socket inside `data/sockets` with
   `listen_addresses=''`, so there is no TCP port open at all.
   On Windows it uses loopback at a free port the OS chooses, because Windows
   Postgres has no socket transport.
3. Starts the bus on loopback at a free port the OS chooses.
4. Runs migrations with the owner role.
5. Starts `api` and `worker` on free ports the OS chooses.
6. Serves the SPA and passes the `api` paths on, on **one fixed port**.
7. On `Ctrl-C`, stops everything, last started first, and stops Postgres
   safely.

Each child gets its working folder pinned to the installation folder. The
`password_file` for the first admin is written as a path from the folder, so
the folder can still move. A path like that resolves from the working folder of
the child, instead of from the place where the user started it.

### One fixed port, and free ports for the rest

Only the UI port is fixed: 8800 by default, and `MARGINCE_PORT` changes it.
The browser is the only way in, and a link the user keeps cannot follow a port
that changes every time the app starts. For the same reason, a port already in
use is refused, instead of moved silently. The `api`, the bus and (on Windows)
the database use free ports the OS chooses, because nothing outside the folder
needs to reach them.

The launcher serves the SPA itself and passes on the `api` paths, using the
same list that `frontend/vite.config.ts` passes on in dev. One origin means no
CORS setup that the server has no other reason to carry. It also keeps the
`api` port private.

### Stopping asks for the fast way, on each platform

Postgres reads `SIGTERM` as a *smart* stop, and waits for every client to
leave. That never happens while a pooled connection is open, so the app would
never stop. `SIGINT` is the fast stop: open transactions are dropped, then the
server stops safely. `SIGQUIT` would be faster, but it leaves repair work for
the next start. A stop that is not safe, every time the app ends, puts the
desktop database at more risk.

Windows has no signals. So the same request is written as `pg_ctl stop -m fast`
for the database, and as a `CTRL_BREAK` console event for everything else. Each
child starts in its own process group. A control event sent to the console
reaches every process on that console. So without the groups, the supervisor
would kill itself on the way to killing its first child.

The Go runtime maps
that event to `os.Interrupt`, which `cmd/api` and `cmd/worker` already wait on.
So the shipped binaries stop through the same path they use on a server.

## Where the two platforms are different

Four things must be different. Everything else is shared.

### The database is reached in different ways, so it is kept safe in different ways

macOS uses `trust` auth over a unix socket in a `0700` folder. No password is
sent, and the file system is the access control. For one user on one Mac, that
is safer than a password stored beside the data it guards.

Windows Postgres has no socket transport at all. So the cluster takes
connections on loopback, and `trust` auth there would open the database to
every other account on the machine. So on Windows the launcher makes a password
per role, and sets up the cluster with `scram-sha-256`. It gives `initdb` the
secret in a file, because every process on the machine can read a command line.

With no socket, there is also no 103-byte path limit. So a Windows installation
may live in any place the user put it.

### Postgres cannot be a child process on Windows

`postgres.exe` refuses to start under an account with admin rights. It says
`Execution of PostgreSQL by a user with administrative permissions is not permitted`.
`pg_ctl` is what creates the limited process that drops those
rights. Starting `postgres.exe` directly would work for a normal account and
fail for an admin. That split only shows up on the machine of someone else.

The cost is that the Postgres server process is not a child of the launcher.
So a launcher that is killed, not stopped, can leave one holding the data folder.
The next start makes up for it: it asks `pg_ctl status` and stops an old one
before it starts. Without that, it would fail at every start, with an error about the
data folder that the user cannot read.

### The event bus is Valkey on macOS and Redis on Windows

macOS ships **Valkey**. This binary ships inside a BUSL-1.1 product, and Redis
7.4 and later is under `RSALv2/SSPL`. Valkey is the fork of the same line under
a BSD license.

Valkey has no Windows build. The Valkey project will not add one, and points
Windows users at WSL. A bundle that promises nothing to install first cannot
ask for WSL. So Windows ships **Redis 7.2**, the last `BSD-3` line before the
license change, and the line Valkey forked from. It may be shipped on the same
license as before, and it works with the `platform/events` code as it is.

Two other choices fail. The old Windows ports of Redis stop at 5.0, and the
outbox reader uses `XAUTOCLAIM`, which was added in 6.2. Those builds would
fail on the first message that waits too long, instead of at build time.
Garnet from Microsoft is MIT and runs on Windows directly, but it has no stream
commands at all. So `events.Relay` would have nothing to write to.

The last choice has a license cost. No Redis is built with MSVC alone, because
Redis needs `fork()`, unix socket support and event handling that Windows does
not have. Every working Windows build gets them from a POSIX layer. That layer
ships as `msys-2.0.dll` beside `redis-server.exe`, and the DLL is under
`LGPLv3`. It ships with no changes and with its license text, which is what the
license asks for. The build step does the copy, so the license is followed in
code.

### Signing

macOS must sign. It refuses to run a binary whose signature is wrong, which is
why the move step signs every file it changes again. Windows has no such rule,
and Authenticode needs a signing key that costs money, not a free one. So the
Windows bundle is not signed, and the first start shows a SmartScreen warning.
The how-to explains the warning; the build does not work around it.

## Settings

`margince.env` is the one place where features are turned on. The first run
writes it with every supported setting explained and commented out. So it also
serves as the reference for what can be turned on. The full field list is in
[reference/configuration.md](../reference/configuration.md).

Its content becomes the environment of `api` and `worker`. That is the same
settings surface a server sets through its environment. Here a file supplies
it, because a desktop installation has no hosting system to set it. These rules
hold:

- `MARGINCE_ENV` is `dev` by default, and `margince.env` may change it. This is
  the one place the desktop shape and the server shape disagree, and the reason
  is the license. A serving role boots on a license or it does not boot, and
  `MARGINCE_ENV` fails closed. So an installation that names nothing is
  production, and it must hold a license nobody gave it. Pinned to production,
  the bundle could not start at all.

  The `dev` mode is safer than it looks. It does not turn on the admin
  endpoint that erases all data. A setting in `margince.yaml`,
  `operations.allow_data_reset`, gates that endpoint, and the first run never
  writes it. So the route stays a 404 whatever the mode says. The mode does
  make `/me` report `non_production`, and it would accept a license made by a
  test authority. This installation has neither.

  It is a default, not a decision. An operator who has a license puts
  `MARGINCE_LICENSE` and `MARGINCE_ENV=production` in `margince.env`, and the
  installation must then meet both. The database connection strings, on the
  other hand, are still added after the settings of the user, and nothing can
  take their place.
- A wrong line refuses the start, and names the file and the line. If a setting
  with a typing error were skipped silently, a user would think a feature does
  not work.

Secrets live in this file at `0600`, not in the Keychain or the Windows
credential store. On Windows the `0600` mode does nothing; see the known limits
below.

## Known limits

- On macOS, a folder with too long a path cannot start. `sockaddr_un` limits a
  socket path to 103 bytes. Since every path starts in the folder, the length
  of the folder path decides whether the database can start. There is no
  `/tmp` fallback, because that would put runtime state where the user cannot
  see or delete it. The launcher measures the path and says what to do.
- Text order is byte order. Postgres is built `--without-icu` on macOS and set
  up `--no-locale` on both, the only setting that is the same on every platform. Text
  with marks such as `é` is stored and returned correctly, but
  `ORDER BY full_name` orders rows by byte value. Users can see this, and
  nobody has decided it yet.
- Neither build is signed for public release. macOS has a local signature only,
  and needs an Apple Developer ID plus notarization. Without them, a copy from
  the web is put in quarantine. Windows is not signed and warns through
  SmartScreen.

  The quarantine mark costs one prompt instead of one per binary, because the
  bundle removes it. A browser marks the zip, and the tool that opens it copies
  the mark onto every file it takes out. Gatekeeper checks at `exec` time. So a
  bundle with the mark still on would stop its own boot 6 times, once per
  program the launcher starts. None of those prompts would explain why the answer to the one
  before did not count.

  `Start Margince.command` clears the mark from the
  launcher, whose own prompt the user has answered. The launcher clears
  `runtime/` before it starts anything. `data/` is never touched: the records
  of the user keep their source mark, and a live socket there would fail the
  call in any case.

  This code works around the missing signature. A build with
  notarization reaches none of it, because Gatekeeper never asks.
- The bus credential is on the command line of the bus. The event bus requires
  a password, made per installation and stored beside the admin password. The
  `api` and the `worker` get it in their child environment, where the database
  connection strings already go. The copy for the bus cannot go that way. The
  bus takes `--requirepass` and no environment setting for it, and a config
  file would be a second place to keep one secret in step.

  So that one value shows in `ps` on this machine. A local account needs the
  credential, and a TCP connection alone is not enough. To remove the last gap,
  the bus would have to accept its password from the environment. The desktop
  bundle cannot do that on its own.
- On Windows, file permissions come from the folder. Go maps the `0600`
  permission value to the read-only flag and sets no DACL. So `margince.env`,
  `data/admin-password` and the database passwords the launcher makes all take
  the permissions of the folder they land in. Under a user profile that folder
  is already per account, and the files are private. In a shared place such as
  `C:\Margince`, every local account on the machine can read them. That
  includes the database password that `postgres_windows.go` uses as the access
  control.

  A fix means setting a DACL by hand. The built-in Go packages do not expose
  that, and this launcher imports nothing else, so it does not do it. Keep a
  Windows installation inside your own user folder.
- One architecture per build, and it is the one the builder has. An Apple
  silicon bundle will not start on an Intel Mac. An Intel one runs on Apple
  silicon only under Rosetta 2. Windows is `x64` with no ARM build. To ship to
  machines of both kinds, build once per architecture.
- The Windows build expects the Microsoft Visual C++ runtime. Most software
  built with MSVC installs it for the whole machine. The C++ tools that the
  build requires put it on the build host. So the check that runs binaries from
  the built folder cannot see that it is missing. A machine without it fails at
  the first `postgres.exe`, with a prompt that names a missing DLL.
- Windows gets no timezone. Windows stores its own timezone ID. The map to the
  IANA name that `margince.yaml` takes lives in CLDR data, and no built-in Go
  package exposes it. So the first run writes `UTC`, and the user fixes it in
  one line. The bundle does not carry a copy of that table.
- No object store ships in the bundle. MinIO changed its license to `AGPLv3`,
  which is a problem to ship inside a BUSL-1.1 product, so no `S3` server ships
  in the folder. Files users add, such as an attachment or a company logo, still
  work: the launcher points the file system provider of `blobstore` at
  `data/blobs`. An installation on a single machine needs nothing more. A local
  `S3` server that only writes to this machine would add a step that the seam
  does not need.

  So the folder has none of what a shared store gives. It keeps no copies on
  other machines, no versions and no signed links, and it shares nothing
  between machines.
  `MARGINCE_BLOBSTORE_ENDPOINT` still wins for an installation that has a real
  store.
- The `api` gives no warning at start about features it cannot use. Connectors
  and webhooks that are not set up log nothing at start. So a user who never
  opens `margince.env` gets no signal.
- A launcher that is killed, not stopped, leaves its child processes running, on both
  platforms. The fixed UI port is then still in use, and the next start is
  refused and names the port. Only the Windows database fixes itself.
- There is no way to back up or to put a copy back. There is no way to move
  the data to a newer Postgres release line, and no guide on first run.

## Status

Proof of concept. The macOS build boots, runs its migrations, serves the UI,
keeps its data after a second start, and stops safely. The Windows lane is
written against what the platform docs say. It has not yet run end to end on a
Windows host, and the limits above apply on both.
