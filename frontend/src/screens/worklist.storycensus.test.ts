// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { KNOWN_SOURCES } from "./worklist.copy";

// Every source the queue can draw has a story that draws it.
//
// A story is where a person LOOKS at a row. The row for a source nobody has a
// story for is one nobody has seen outside the running product — and this tree
// has shipped two rows that were wrong in ways a glance would have caught: a
// brief item offering three verbs the client drew none of, and a failed
// automation with no address at all.
//
// Neither was a logic error a unit test would find. Both were "the row is
// there and it is useless", which is exactly what a story shows and an
// assertion does not.
//
// THE CORPUS IS DERIVED, from KNOWN_SOURCES rather than a list here. A second
// copy of the vocabulary is the thing that goes stale, and a census reading a
// shorter world reports PASS with nothing to notice.

const storiesDir = join(__dirname);

// `batch` names no single record — it stands for a pile of other rows, and its
// screen and verbs are its members'. There is nothing source-shaped to draw.
const standsForOtherRows = "batch";

function everySourceDrawnInAStory(): Set<string> {
  const drawn = new Set<string>();
  // The WORKLIST's own stories, not every story in the directory. `source` is
  // an ordinary field name — an AI call's story names a `source: "heuristic"`,
  // a verdict's a `source: "human"` — and a sweep over all of them reads two
  // dozen words from vocabularies this census knows nothing about.
  for (const file of readdirSync(storiesDir)) {
    if (!file.startsWith("worklist.") || !file.endsWith(".stories.tsx")) {
      continue;
    }
    const text = readFileSync(join(storiesDir, file), "utf8");
    for (const match of text.matchAll(/source: "([a-z_]+)"/g)) {
      drawn.add(match[1]);
    }
  }
  return drawn;
}

describe("every source the queue draws has a story", () => {
  const sources = Object.keys(KNOWN_SOURCES).filter(
    (source) => source !== standsForOtherRows,
  );

  // The corpus must not be able to come back empty or short. A glob that
  // matched no files, or a KNOWN_SOURCES that stopped being read, would leave
  // every assertion below vacuously true.
  it("reads a real vocabulary and a real set of stories", () => {
    expect(sources.length).toBeGreaterThan(15);
    expect(everySourceDrawnInAStory().size).toBeGreaterThan(15);
  });

  it.each(sources)("draws %s", (source) => {
    expect(everySourceDrawnInAStory()).toContain(source);
  });

  // And nothing storied that the queue can no longer draw, which reads to the
  // next author as though that source still existed.
  it("draws nothing the queue no longer knows", () => {
    const known = new Set(Object.keys(KNOWN_SOURCES));
    const stale = [...everySourceDrawnInAStory()].filter(
      (source) => !known.has(source),
    );
    expect(stale).toEqual([]);
  });
});
