// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { ENTITY_KINDS } from "../app/entity";
import { recordWriteKeys } from "./recordwritekeys";

describe("recordWriteKeys", () => {
  it("answers for every record kind the history panel can be opened on", () => {
    for (const kind of ENTITY_KINDS) {
      expect(recordWriteKeys(kind, "r-1").length).toBeGreaterThan(0);
    }
  });

  it("names every sibling read of a contact, not one of the three", () => {
    // The three are siblings under no common prefix, so invalidating one
    // leaves the other two painting the state the reader just changed.
    expect(recordWriteKeys("person", "p-1")).toEqual([
      ["person", "p-1"],
      ["person360", "p-1"],
      ["personBrief", "p-1"],
    ]);
  });

  it("carries the lead's board and list, which its detail key does not reach", () => {
    expect(recordWriteKeys("lead", "l-1")).toContainEqual(["leads"]);
  });
});

// The defect is invisible to a test of the module itself: it is a call site
// that never used it. Every restore callback in this tree hand-spelled ONE
// key, and each picked a different one, so the gate is derived from the tree
// rather than from a list kept beside it.
//
// A generous budget, stated rather than defaulted, as every gate that parses
// the whole tree declares: this is synchronous CPU work over several hundred
// files, about two seconds alone and past vitest's default ten under a loaded
// runner. There is no hang for a tight timeout to catch, so the ceiling is the
// floor under a slow runner and nothing else.
const scanBudget = { timeout: 60_000 };

describe("every restore callback goes through the helper", scanBudget, () => {
  it("finds no onRestored that invalidates by hand", () => {
    const offenders: string[] = [];
    for (const file of sourceFiles()) {
      const text = readFileSync(file, "utf8");
      const parsed = ts.createSourceFile(
        file,
        text,
        ts.ScriptTarget.Latest,
        true,
      );
      const visit = (node: ts.Node): void => {
        if (
          ts.isPropertyAssignment(node) &&
          ts.isIdentifier(node.name) &&
          node.name.text === "onRestored" &&
          node.initializer.getText().includes("invalidateQueries")
        ) {
          offenders.push(
            `${relative(SRC, file)}: ${node.initializer.getText().replace(/\s+/g, " ")}`,
          );
        }
        ts.forEachChild(node, visit);
      };
      visit(parsed);
    }
    expect(
      offenders,
      "a restore callback spelling its own query key invalidates one read of a " +
        "record that is served under several; call invalidateRecord instead",
    ).toEqual([]);
  });
});

const SRC = join(dirname(fileURLToPath(import.meta.url)), "..");

// Stories and tests supply their own no-op callbacks and are not writers.
function sourceFiles(): string[] {
  const found: string[] = [];
  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(path);
        continue;
      }
      if (!/\.tsx?$/.test(entry.name)) {
        continue;
      }
      if (/\.(test|stories)\.tsx?$/.test(entry.name)) {
        continue;
      }
      found.push(path);
    }
  };
  walk(SRC);
  return found;
}
