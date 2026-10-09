<!-- prose:plain -->
# LICENSE release rule: setting the Change Date

**Owner:** Legal (Hà Trần Minh) | **Run by:** the member who makes a release
**Agreed:** 2026-07-05

## The rule

Every tagged release carries its own values in the LICENSE `Parameters` block, set before the tag
goes public:

- **Change Date** is the date the release goes public + 2 years, ISO format
  (`YYYY-MM-DD`).
- The **Licensed Work** line names the version, for example
  `Licensed Work: Margince CRM v1.3.0`.

All text from the first `---` line after the `Parameters` block to the end of
the file is the fixed BUSL-1.1 body. Never change it (BUSL Covenant 4).

The release step that sets them is in [Cut a release](../how-to/cut-a-release.md).

## What counts as a "Release"

A **Release** is a version with a git tag and a GitHub Release, both public. A
commit or a branch push on its own is not a release and does not carry its own
Change Date. The BUSL body allows each version of the Licensed Work its own Change
Date. This rule is how we use that part of the license.

## Why the date counts

- The README makes a public promise: every release turns into Apache 2.0 two
  years after it ships. An old date typed into the file breaks that promise in
  two ways. Later releases turn into Apache 2.0 before two years have passed,
  which gives up the window the license keeps for business use. And a release
  that carries a past date is Apache 2.0 at once.
- If a release ships without the update, the four-year default in the BUSL body
  applies to that version. That is legally safe, wrong for the business, and
  does not match the public promise in the README.

## How it is checked

Not checked yet: no workflow or gate checks the Change Date
([#6840](https://github.com/margince/margince/issues/6840)). The check to add
runs on a `v*` tag. It fails when the `Change Date` in LICENSE is not the tag
date plus two years. It also fails when the release changes any line below
the `Parameters` block.
