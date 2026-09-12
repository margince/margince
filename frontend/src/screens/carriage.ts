// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The carriage bounds a channel reply is held to BEFORE it is sent, checked
// against the numbers `GET /channel-providers` publishes for the resolved
// transport.
//
// The AUTHORITY is the dispatcher's own `carriageRefusal` (backend
// `comms/gates.go`): it parks a delivery that breaks any of these, and it is
// the only thing that can, because nothing enforces a request body against the
// schema. This is an advisory pre-check so the block lands in front of the rep
// rather than after staging. The two cannot share a helper across the wire, and
// they cannot drift into a wrong SEND: a client that fell behind the server
// would only fail to warn, and the server would still refuse. What it must
// never do is run AHEAD of the server and block a send the server would take —
// which is why it reads the published bounds rather than a second copy of them.
// `max_total_bytes` already arrives as the EFFECTIVE limit (the smaller of the
// transport's own aggregate and the product budget), so this never respells
// `MaxSendBytes` — the two-spellings drift that was #2047.

import type { components } from "../api/schema";
import type { ChosenFile } from "./composeattachments";

type Carriage = components["schemas"]["ChannelProviderEntry"]["attachments"];

// One reason a message may not go as attached. Structured rather than a
// sentence so the caller owns the wording, and a test can assert the bound that
// bit rather than the English it produced.
export type CarriageViolation =
  | { kind: "carries"; count: number }
  | { kind: "count"; limit: number; named: number }
  | { kind: "perFile"; filename: string; limit: number }
  | { kind: "aggregate"; total: number; limit: number; count: number }
  | { kind: "caption"; limit: number; length: number };

// carriageViolations mirrors `carriageRefusal`'s checks in its order, and
// returns every bound this message breaks rather than only the first: the
// server parks on one because a park needs one reason, but a rep fixing them one
// round trip at a time is the late-and-correct experience this pre-check exists
// to end.
//
// `carries: false` SUPERSEDES the rest — a transport that takes no files at all
// has no per-file or aggregate bound to also report, and listing them would
// offer a rep a fix ("send fewer") for a wall ("send none").
//
// A file whose size the shelf does not know is NOT counted against a byte
// bound. An unmeasured file is not a state the record reaches — every writer
// records a real size — and guessing would either warn falsely or, worse, run
// ahead of the server and block a send it would take.
export function carriageViolations(
  carriage: Carriage,
  files: readonly ChosenFile[],
  body: string,
): CarriageViolation[] {
  if (files.length === 0) {
    return [];
  }
  if (!carriage.carries) {
    return [{ kind: "carries", count: files.length }];
  }
  const violations: CarriageViolation[] = [];
  if (carriage.max_files > 0 && files.length > carriage.max_files) {
    violations.push({
      kind: "count",
      limit: carriage.max_files,
      named: files.length,
    });
  }
  for (const file of files) {
    if (
      carriage.max_bytes_per_file > 0 &&
      file.byteSize != null &&
      file.byteSize > carriage.max_bytes_per_file
    ) {
      violations.push({
        kind: "perFile",
        filename: file.filename,
        limit: carriage.max_bytes_per_file,
      });
    }
  }
  // The aggregate is always present and always positive, so unlike the others
  // it has no "no bound" reading. Only measured files count toward it.
  const total = files.reduce((sum, file) => sum + (file.byteSize ?? 0), 0);
  if (total > carriage.max_total_bytes) {
    violations.push({
      kind: "aggregate",
      total,
      limit: carriage.max_total_bytes,
      count: files.length,
    });
  }
  // The caption bound applies to the body only once a file is attached, which
  // the length check above already established. Runes, not UTF-16 units, to
  // agree with the server's `utf8.RuneCountInString`: an emoji is one character
  // to the contact counting and one rune to the gate.
  const length = [...body].length;
  if (
    carriage.max_body_with_files > 0 &&
    length > carriage.max_body_with_files
  ) {
    violations.push({
      kind: "caption",
      limit: carriage.max_body_with_files,
      length,
    });
  }
  return violations;
}
