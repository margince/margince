# LICENSE release rule: Change Date stamping

**Owner:** Legal (Hà Trần Minh) | **Executed by:** whoever cuts a release
**Ratified:** 2026-07-05

## The rule

On every tagged release, before the tag is published, update the LICENSE
Parameters block and nothing else in the file:

1. **Change Date** = the release's publication date + 2 years, ISO format
   (`YYYY-MM-DD`).
2. The **Licensed Work** line gains the version identifier, e.g.
   `Licensed Work: Margince CRM v1.3.0`.

Everything from the first `---` separator after the Parameters block to the end
of the file is the canonical BUSL-1.1 body and must never be edited
(BUSL Covenant 4).

## What counts as a "Release"

A **Release** is a version tagged and published via a git tag / GitHub Release.
Individual commits and branch pushes are not releases and do not carry their
own Change Date. The BUSL body applies "separately for each version of the
Licensed Work and the Change Date may vary for each version"; this rule is how
we use that mechanism.

## Why the date matters

- The README publicly promises every release converts to Apache 2.0 two years
  after it ships. A stale hard-coded date breaks that promise in one of two
  directions: later releases convert earlier than two years (giving away the
  commercial window), and any release still carrying a past date is Apache 2.0
  immediately.
- If a release ships without the update, the BUSL body's four-year backstop
  applies to that version. That is legally safe, commercially wrong, and publicly
  inconsistent with the README.

## Enforcement

Not enforced yet: no workflow or gate checks the Change Date
([#6840](https://github.com/margince/margince/issues/6840)). The check to add:
on a `v*` tag, fail when LICENSE's `Change Date` is not the tag date plus two
years, or when the diff touches anything below the Parameters block.
