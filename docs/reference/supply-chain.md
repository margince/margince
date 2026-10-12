<!-- prose:plain -->
# SBOMs, license gate and signatures

An **SBOM** (`software bill of materials`) is a list, for machines to read, of all
the tree ships: every dependency, its version, and its license. Three tools run
this lane, all as Docker images pinned by hash: **syft** generates the SBOMs,
**grant** gates their licenses, and **cosign** signs them.

```text
git archive HEAD ─▶ syft ─▶ 3 SBOM docs ─▶ normalize ─▶ parity ──┬─▶ grant (license gate)
                                                                 ├─▶ validate (per-format)
                                                                 └─▶ cosign (main only,
                                                                     needs parity + validate)
```

The SBOM lane covers the whole repository (`backend/`, `frontend/` and `extensions/`), so
its targets live in the root `Makefile`, outside `backend/`. It is not part of
`make check`: CI runs it as its own workflow, `.github/workflows/sbom.yml`, and
only when someone starts it by hand. The dependency license policy still runs on
its own on every PR that touches a dependency, as the `license gate` job in
`ci.yml`.

All of this describes the source tree, not a container image: the scan input is
the committed content of `HEAD`.

## What it makes

`make sbom` exports `git archive HEAD` into `.tmp/sbom-src/`, points syft at that
export, and deletes it again on the way out (through a `trap`). It scans an export
and not the working tree. So host state (`node_modules`, `.env`, files your tools write, a
test you did not commit) can never get into an SBOM. The committed content of
`HEAD` is the one true source on what is scanned.

`.gitignore` is not that source. It does not remove a file already tracked in
`HEAD`, and `git add -f` can commit a file that `.gitignore` names.

One scan, three documents:

| File | Format | syft writer |
|---|---|---|
| `sboms/margince.cdx.json` | CycloneDX JSON | `cyclonedx-json` |
| `sboms/margince.spdx221.json` | SPDX JSON 2.2.1 | `spdx-json@2.2` |
| `sboms/margince.spdx300.json` | SPDX JSON 3.0 | `spdx-json@3.0` |

`/sboms/` is in `.gitignore`: these are build files that CI publishes, never
committed.

Scan policy lives in [`.syft.yaml`](../../.syft.yaml). The `Makefile` owns the
"what is scanned" policy (the clean export), and the config owns the rest:

- `source.name: margince`. There is no `source.license` key in syft, so config
  cannot set the license of the top part. The Margince source license (BUSL-1.1) is handled
  on the grant side instead.
- `enrich: [all]` with `license.content: none`. License data comes from the Go
  module proxy and the npm register, so `make sbom` needs network access. It is
  not an offline scan.
- `file.metadata.selection: all`, with `sha1` + `sha256` + `sha512` hash values. The syft
  default (`owned-by-package`) hashes only files that it ties to a package it
  found. That would leave our own source (`backend/**`, `frontend/src/**`,
  migrations, config templates) with no checksum at all. We keep SHA-1 because
  SPDX file entries have always keyed on it; SHA-256 and SHA-512 are what a
  consumer checks.
- **No `exclude:` list.** The release gate refuses a release unless the SBOM
  lists every file the release patch adds or changes. That patch is a diff of the
  full committed tree that leaves nothing out. So the SBOM file set must be the
  whole committed tree.
  - Leaving out any committed tree here (CI workflows, `fixtures`,
    `sbom-schemas`, …) would make a commit that touches it fail that gate.
  - Host state that is not committed is already missing, because the scan runs
    on `git archive HEAD`.

## Versioning

`SBOM_VERSION` goes to syft as `--source-version`, so it goes in each
document and not only in a file name. That puts it under the cosign signature.

```make
SBOM_VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "dev-$$(git rev-parse HEAD 2>/dev/null || echo unknown)")
```

- **`HEAD` on a tag** ⇒ only the tag. A tag maps to one commit, so the commit
  is clear from it.
- **Else** ⇒ `dev-<full git revision>`, so you can trace a published SBOM that
  comes before a release to its commit.
- `--exact-match` is required. Plain `git describe` would use the closest
  earlier tag in its `-N-g<sha>` form instead. This repository carries tags that are not
  releases (`archive/*`), and those would then read as a release version.
- You can set it on the command line (`make sbom SBOM_VERSION=v1.3.0`). The
  LICENSE update that goes with a real tag is a separate rule; see
  [license-release-rule.md](license-release-rule.md).

## The one-tree rule

The three documents must describe one tree. For one scan, the three syft writers
do not agree on the file set by default:

- **CycloneDX** names every file with the full prefix of the scan root
  (`/src/.tmp/sbom-src/`); **SPDX** names are relative to the repository.
- Both **SPDX** writers also write an extra entry per folder, whose only checksum
  is the zero SHA-1. They also add one entry with an empty name for the scan root.
  CycloneDX writes no such entries.

`make sbom-normalize` makes them agree. It strips the prefix from CycloneDX file
parts, and drops the entries with an empty name or a zero SHA-1 from both SPDX
documents. That leaves all three listing the same files, relative to the
repository. It runs on the syft output before any signature, so the signed bytes
are the cleaned bytes. Running both filters twice changes nothing. That also
holds for a later syft that already writes relative names and no folder entries.

The zero SHA-1 is the test because it is the only signal syft gives. Version
1.50 labels every SPDX element `software_fileKind == "file"`, folders included,
so the kind field cannot tell a folder from a file. A real file always carries a SHA-256
and SHA-512 that is not zero, next to its SHA-1.

`make sbom-parity` is the check. It pulls out the three sets of file names
(`.components[]|select(.type=="file")|.name`, `.files[].fileName`, and
`.["@graph"][]|select(.type=="software_File")|.name`). It sorts each set, and
compares each set with each other set. Green prints `OK: three SBOMs list the same <n> files`;
any difference prints the diff and fails the build.

A syft upgrade that adds back the scan-root prefix or the folder entries
breaks here, and not later when the release is checked. `make sbom` runs
`sbom-normalize` and then `sbom-parity` itself, and CI runs `make sbom`, so every
CI run is guarded.

## The license gate

`make sbom-check` runs **grant** against the CycloneDX document only:

```bash
grant check sboms/margince.cdx.json -c .grant.yaml
```

[`.grant.yaml`](../../.grant.yaml) sets `require-license: true` and
`require-known-license: true`. So a package with no license found, or one that
grant cannot resolve, is refused, the same as a license that is not allowed.

The allow list, as set up:

| Group | License names |
|---|---|
| Open licenses with few rules | `Apache-2.0`, `MIT`, `BSD-2-Clause`, `BSD-3-Clause`, `ISC`, `0BSD`, `Zlib`, `BSL-1.0`, `CC0-1.0` |
| Found in the tree, added to cover it | `MIT-0`, `BlueOak-1.0.0`, `MPL-2.0` |
| Set by a maintainer | `CC-BY-4.0`, `Python-2.0`, `Unlicense` |
| The own source license of Margince | `BUSL-1.1` |

Two groups never reach the allow list check:

- **Our own packages** are skipped by name and path (`ignore-packages`):
  `github.com/margince/margince/*` (our own Go modules) and
  `example.margince.dev/*` (the committed sample extensions). They carry no
  third-party license to gate.
- **Local composite actions** under `.github/actions/` are skipped by name and
  path too (`./.github/actions/*`). They are our own files and carry the
  repository's own BUSL-1.1.
  - They cannot be handled the way the pinned third-party actions are. A local
    action gets no purl at all from syft, while `make sbom-supplement` keys its
    map on purl. So the map cannot reach them.
  - The entry uses a `*` pattern, so the next composite action is covered on the day
    it is added.

All else must still resolve to a license that grant knows and allows.

## Format check

The parity check proves the three documents describe the same tree. It does not prove that
each one is valid in its own format. So `make sbom-validate` checks each one with
the validator its format has:

| Document | Validator |
|---|---|
| `margince.cdx.json` | `cyclonedx validate --fail-on-errors` |
| `margince.spdx221.json` | `pyspdxtools`, from a requirements file pinned by hash |
| `margince.spdx300.json` | `jsonschema` against `sbom-schemas/spdx-3.0.1.schema.json`, kept in the tree |

`make sbom-sign` depends on `sbom-parity` and `sbom-validate`, so a document
that does not agree with the others, or is not valid, cannot be signed. A
signature over a bad SBOM would make the bad document look trusted. The
SPDX 3.0.1 schema is kept in the tree, not fetched, so the check gives the same
result each time and cannot change under a release. `sbom-schemas/README.md`
records where it came from.

## Signing

`make sbom-sign` signs each of the three documents with cosign, with no key of
its own (`sign-blob --yes --bundle`). It writes one bundle per SBOM:

- `sboms/margince.cdx.json.cosign.bundle`
- `sboms/margince.spdx221.json.cosign.bundle`
- `sboms/margince.spdx300.json.cosign.bundle`

It requires an OIDC token: `SIGSTORE_ID_TOKEN`, or the GitHub
`ACTIONS_ID_TOKEN_REQUEST_URL` / `ACTIONS_ID_TOKEN_REQUEST_TOKEN`, which are
set in a job that holds `id-token: write`. All three go into the container.

The target depends on `sbom-parity` and not on `sbom`, for two reasons.

- The signature must cover clean bytes that agree with each other, so something
  has to check them again. The parity check does that at low cost. It refuses to
  sign a set when the three SBOMs do not list the same files. It does not check
  that the set matches `HEAD`.
- Running *generation* again here would run the syft scan while the signing token
  is in scope. The job split of the CI workflow (below) does not allow that. In
  CI the signing job uses the build output of the generation job instead.

## How the tools run

syft, grant and cosign all run as Docker images pinned by hash. So the host needs
none of them installed. A tag pushed again to the image store cannot replace the tool
that reads the repository, or the one that holds a signing identity. The tags in
the `Makefile` are comments next to the hash; update tag and hash together.

| Setting | Default | Notes |
|---|---|---|
| `SYFT_IMAGE` | `anchore/syft@sha256:1288ea4c…` (`v1.50.0`) | the scanner |
| `GRANT_IMAGE` | `anchore/grant@sha256:17246361…` (`v0.6.8`) | the license gate |
| `COSIGN_IMAGE` | `gcr.io/projectsigstore/cosign@sha256:c77247c9…` (`v2.4.3`) | signing with no key of its own |
| `SYFT` / `GRANT` / `COSIGN` | `docker run --rm -v "$(CURDIR)":/src -w /src <image>` | set the whole command to use the tools on the host: `make sbom SYFT=syft GRANT=grant` |
| `SBOM_VERSION` | tag, else `dev-<revision>` | see [Versioning](#versioning) |
| `SBOM_DIR` | `sboms` | output folder |
| `SBOM_SRC` | `.tmp/sbom-src` | the path of the clean export (created and removed per run) |
| `COSIGN_HOME` | `.tmp/cosign-home` | the cosign `HOME` in the container |

The host still needs Docker, `git`/`tar` (the clean export) and jq (cleaning and
parity run on the host), plus network access to enrich licenses.

cosign runs as the user who starts it, with a moved `HOME`. The cosign image
defaults to uid 65532. That uid does not own the `sboms/` folder shared into the
container, and has no home folder it can write to. So the command adds `-u $(id -u):$(id -g)` and
`HOME=/src/.tmp/cosign-home`:

- **uid:** cosign writes each `*.cosign.bundle` with mode `0600`. Written as uid
  65532, those bundles would be closed to the next tool that reads them. As the user
  who started it, they stay open to read (in CI, `upload-artifact` runs as the same
  runner, which is not root).
- **`HOME`:** the TUF cache of sigstore needs a place to land. Pointing it at
  `.tmp/`, which is in `.gitignore`, keeps it out of the tree.

## CI: `.github/workflows/sbom.yml`

Builds the SBOMs again, gates their licenses and signs them. It runs only when
someone starts it by hand, with no trigger that runs on its own. The runner needs only
Docker, which `ubuntu-latest` has installed.

| Trigger | When |
|---|---|
| `workflow_dispatch` | by hand, any `ref` (but see the own guard of the `sign` job) |

There is no path filter, because there is no filtered trigger to apply it to.

### Why dispatch only

Every run signs into the public Rekor log, where a signature with no key of its
own can never be removed. And this repository has no releases for a consumer to
fetch.
Signed files describe a release, and a release needs only one start by hand. Runs on
every dependency change would also use up the shared runner limit that the PR
gates wait in.

Running only by hand costs nothing in license checks. The gate that decides
whether a dependency may land is `license gate` in `ci.yml`. It is gated at job
level on the `deps` scope, and `main` only gets a dependency change through a PR
that passed it.

That gate lives in `ci.yml`, because a `paths:` filter at workflow level writes no
check run when it does not match. A required check that never reports blocks a
merge for good, so a gate that must be required cannot live behind one. Gating at
job level reports a skipped path as passing. See
[ci-pipeline.md](../explanation/ci-pipeline.md).

`permissions: contents: read` at workflow level is the floor for every job. The
right to mint an OIDC token is not granted there. Only the `sign` job asks for
it, so generation code that a branch controls can never mint a signing token.

**Job `sbom`** checks out with `persist-credentials: false` and `fetch-depth: 0`
(`--exact-match` needs tags and history to resolve). It then runs `make sbom`, then
`make sbom-check`. The license gate stays on this path because `sign` below needs
it to pass first. It publishes the `sboms` output from `sboms/`, with
`if-no-files-found: error` and 90-day retention.

**Job `sign`** has `needs: sbom`, and runs only when the dispatch `ref` is
`refs/heads/main` (`if: github.ref == 'refs/heads/main'`). It adds
`id-token: write` to `contents: read` and fetches the `sboms` output; it does
not build it again. It runs `make sbom-sign` and publishes
`sboms/*.cosign.bundle` as the `sbom-signatures` output (retention 90 days).

It is a separate job that never runs off `main`, for two reasons:

1. **A keyless signature is permanent.** It goes into the
   public Rekor log, and no one can remove it. A run from a feature branch must never
   make one.
2. **Branch code stays out.** Generation runs what the `Makefile` and
   `.syft.yaml` of the branch say. With that code kept out of any job that holds
   `id-token: write`, it cannot reach the signing identity.
   - `needs: sbom` also means the license gate has already passed. So an SBOM that
     fails the policy never reaches signing.
   - That is why the gate stays in this workflow, while `ci.yml` also gates
     every PR.

## Targets

| Target | What it does |
|---|---|
| `sbom` | Generate the three SBOMs of the source tree from a clean `git archive HEAD` export into `sboms/`, with license data (needs network). Then run `sbom-normalize` and `sbom-parity`. Signing is not part of it |
| `sbom-normalize` | Make the three syft writers agree on one file set: strip the full scan-root prefix from CycloneDX, and drop the extra SPDX entries for folders and empty names. Running it twice changes nothing; it runs before any signature, so the signed bytes are the cleaned bytes |
| `sbom-parity` | Check that all three SBOMs list the same set of files, relative to the repository; prints `OK: three SBOMs list the same <n> files` or fails with the diff |
| `sbom-check` | The license gate: `grant check sboms/margince.cdx.json -c .grant.yaml`. Refuses a license that is not allowed, unknown or missing; our own packages, local composite actions included, are skipped by name and path |
| `sbom-sign` | cosign `sign-blob` per SBOM, with no key of its own → `*.cosign.bundle`. Needs an OIDC token. Depends on `sbom-parity`, not `sbom`, so it checks again and does not build again; generation never runs under the signing token |

Each of these but `sbom` fails at once with
`FAIL: no SBOM found — run 'make sbom' first` when `sboms/margince.cdx.json` is
missing.

See also:

- [make-targets.md](make-targets.md) for the rest of the root `Makefile`;
- [deploy-margince.md](../how-to/deploy-margince.md) for what ships;
- [license-release-rule.md](license-release-rule.md) for the LICENSE update that a tagged release needs.
