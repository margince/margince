// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
} from "../../scripts/lib/source-tree";

// EVERY SURFACE THAT SENDS SAYS WHAT THE MESSAGE WOULD MEET.
//
// The composer used to be the only place that asked the engine, and it asked
// late: a rep learned a send was refused by pressing Send and reading an error.
// That is fixed, and the fix is one file deep. Nothing stops a second surface —
// a bulk action, a follow-up card, an extension's own drawer — from posting a
// send with no mark beside it and reintroducing exactly that silence.
//
// So the rule is stated here rather than remembered: a file that POSTs a send
// renders the status, or names itself below with why it does not.
//
// WHY A CENSUS AND NOT A REVIEW. The failure is invisible at the call site.
// `api.POST("/emails", …)` reads identically whether the surface around it
// tells the rep anything or not, and the reviewer who would notice is the one
// who already knows this rule. A list that fails is what carries it to somebody
// who does not.

const SCREENS = __dirname;
const EXTENSIONS = join(__dirname, "..", "..", "..", "extensions");

// The three doors a send goes out of. Read as literals rather than derived from
// the generated schema, because what matters is the STRING a caller typed: a
// surface that built the path some other way is a surface this census cannot
// see either way, and pretending otherwise would be worse than saying so.
const SEND_PATHS = [
  '"/emails"',
  '"/activities/{id}/send-email"',
  '"/activities/{id}/send-message"',
];

// What counts as saying something. Any of these means the file draws the
// engine's answer where the rep can see it before they press.
const SAYS_SOMETHING = [
  "SendMark",
  "SendPermission",
  "CommunicationStatus",
  "useSendPermission",
];

// Files that post a send and draw no mark, each with the reason.
//
// EMPTY, and that is the point of landing this now rather than later. The tree
// is clean today, so every entry added here is a deliberate exception somebody
// wrote a sentence for — where a census introduced after the first violation
// would have started with that violation already ratified.
const SILENT_SENDERS: Record<string, string> = {};

function sourceFiles(): string[] {
  return [...filesUnder(SCREENS), ...extensionFrontendFiles(EXTENSIONS)].filter(
    (path) => !/\.(test|stories)\.tsx?$/.test(path),
  );
}

function postsASend(source: string): boolean {
  return SEND_PATHS.some(
    (path) => source.includes(`api.POST(${path}`) || source.includes(path),
  );
}

describe("every surface that sends says what the message would meet", () => {
  it("finds the senders it is meant to judge", () => {
    // A census that stopped finding its subject would report a clean tree over
    // one full of silent senders. This is the arity guard every census owes:
    // the composer posts all three doors, so at least one file must match.
    const senders = sourceFiles().filter((path) =>
      postsASend(readFileSync(path, "utf8")),
    );
    expect(senders.length).toBeGreaterThan(0);
  });

  it("draws the engine's answer beside every send", () => {
    const silent: string[] = [];
    for (const path of sourceFiles()) {
      const source = readFileSync(path, "utf8");
      if (!postsASend(source)) continue;
      if (SAYS_SOMETHING.some((marker) => source.includes(marker))) continue;
      const relative = path.slice(path.indexOf("/src/") + 1);
      if (SILENT_SENDERS[relative]) continue;
      silent.push(relative);
    }
    expect(silent).toEqual([]);
  });

  it("keeps no ratified exception that has stopped being one", () => {
    // A waiver outliving its file is a sentence describing a surface nobody can
    // find, and it makes the next reader trust a list that is already wrong.
    const present = new Set(
      sourceFiles().map((path) => path.slice(path.indexOf("/src/") + 1)),
    );
    for (const named of Object.keys(SILENT_SENDERS)) {
      expect(present.has(named)).toBe(true);
    }
  });
});
