// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { KNOWN_SOURCES } from "./worklist.copy";

// Every source the queue can draw has a story that draws it.
//
// A story is where a contact LOOKS at a row. The row for a source nobody has a
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
  const files = readdirSync(storiesDir)
    .filter(
      (file) => file.startsWith("worklist.") && file.endsWith(".stories.tsx"),
    )
    .map((file) => join(storiesDir, file));
  const configPath = join(storiesDir, "../../tsconfig.app.json");
  const config = ts.readConfigFile(configPath, ts.sys.readFile);
  const parsed = ts.parseJsonConfigFileContent(
    config.config,
    ts.sys,
    join(storiesDir, "../.."),
  );
  const program = ts.createProgram(files, parsed.options);
  const checker = program.getTypeChecker();
  const drawn = new Set<string>();
  function visit(node: ts.Node): void {
    if (
      ts.isPropertyAssignment(node) &&
      node.name.getText() === "source" &&
      ts.isObjectLiteralExpression(node.parent)
    ) {
      // Contextual Partial<WorklistItem> overrides count too. A verdict's
      // provenance has no category, so it cannot masquerade as a queue source.
      const owner =
        checker.getContextualType(node.parent) ??
        checker.getTypeAtLocation(node.parent);
      const source = checker.getTypeAtLocation(node.initializer);
      const hasCategory = node.parent.properties.some(
        (property) => property.name?.getText() === "category",
      );
      if (
        (hasCategory || owner.getProperty("category")) &&
        source.isStringLiteral()
      )
        drawn.add(source.value);
    }
    ts.forEachChild(node, visit);
  }
  for (const file of files) {
    const source = program.getSourceFile(file);
    if (!source) throw new Error(`Story was not loaded: ${file}`);
    visit(source);
  }
  return drawn;
}

const drawnSources = everySourceDrawnInAStory();

describe("every source the queue draws has a story", () => {
  const sources = Object.keys(KNOWN_SOURCES).filter(
    (source) => source !== standsForOtherRows,
  );

  // The corpus must not be able to come back empty or short. A glob that
  // matched no files, or a KNOWN_SOURCES that stopped being read, would leave
  // every assertion below vacuously true.
  it("reads a real vocabulary and a real set of stories", () => {
    expect(sources.length).toBeGreaterThan(15);
    expect(drawnSources.size).toBeGreaterThan(15);
  });

  it.each(sources)("draws %s", (source) => {
    expect(drawnSources).toContain(source);
  });

  // And nothing storied that the queue can no longer draw, which reads to the
  // next author as though that source still existed.
  it("draws nothing the queue no longer knows", () => {
    const known = new Set(Object.keys(KNOWN_SOURCES));
    const stale = [...drawnSources].filter((source) => !known.has(source));
    expect(stale).toEqual([]);
  });
});
