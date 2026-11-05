// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  parseSource,
  sourceFileAt,
  sourceTextAt,
} from "../../scripts/lib/source-tree";

// Whose move a message waits on is the server's answer (moveLabel). A branch
// on `direction` that yields a move label is a second rule beside it, and it
// told readers a colleague's calendar invitation needed a reply.

const DIR = join(__dirname);
const MOVE_KEY = /^(email\.move\.|convo\.)/;

function readsDirection(node: ts.Node): boolean {
  let found = false;
  const visit = (child: ts.Node): void => {
    if (
      (ts.isPropertyAccessExpression(child) &&
        child.name.text === "direction") ||
      (ts.isIdentifier(child) && child.text === "direction")
    ) {
      found = true;
    }
    ts.forEachChild(child, visit);
  };
  visit(node);
  return found;
}

function yieldsMoveLabel(node: ts.Node): boolean {
  let found = false;
  const visit = (child: ts.Node): void => {
    if (ts.isStringLiteralLike(child) && MOVE_KEY.test(child.text)) {
      found = true;
    }
    if (
      ts.isCallExpression(child) &&
      child.expression.getText() === "moveLabel"
    ) {
      found = true;
    }
    ts.forEachChild(child, visit);
  };
  visit(node);
  return found;
}

/** The statements in one source that choose a move label from a direction. */
function directionMoveBranches(name: string, source: string): string[] {
  const file = parseSource(name, source);
  const out: string[] = [];
  const visit = (node: ts.Node): void => {
    let condition: ts.Node | undefined;
    let branches: ts.Node[] = [];
    if (ts.isConditionalExpression(node)) {
      condition = node.condition;
      branches = [node.whenTrue, node.whenFalse];
    } else if (ts.isIfStatement(node)) {
      condition = node.expression;
      branches = [node.thenStatement];
    } else if (ts.isSwitchStatement(node)) {
      condition = node.expression;
      branches = [...node.caseBlock.clauses];
    }
    if (
      condition &&
      readsDirection(condition) &&
      branches.some(yieldsMoveLabel)
    ) {
      const { line } = file.getLineAndCharacterOfPosition(node.getStart());
      out.push(`${name}:${line + 1}`);
    }
    ts.forEachChild(node, visit);
  };
  visit(file);
  return out;
}

function designSystemSources(): string[] {
  return readdirSync(DIR).filter(
    (name) => /\.tsx?$/.test(name) && !/\.(test|stories)\.tsx?$/.test(name),
  );
}

describe("no direction decides a move label", () => {
  it("finds none in the design system", () => {
    const sources = designSystemSources();
    for (const expected of [
      "timelineentry.tsx",
      "timelinethread.tsx",
      "emailentry.tsx",
      "emaildetail.tsx",
    ]) {
      expect(sources).toContain(expected);
    }
    const directionReaders = sources.filter((name) =>
      readsDirection(sourceFileAt(join(DIR, name))),
    );
    expect(directionReaders.length).toBeGreaterThan(0);
    const found = sources.flatMap((name) =>
      directionMoveBranches(name, sourceTextAt(join(DIR, name))),
    );
    expect(found).toEqual([]);
  });

  it("reports a planted direction rule and spares a direction sentence", () => {
    expect(
      directionMoveBranches(
        "planted.tsx",
        `const flag = entry.direction === "inbound" ? t("email.move.needsReply") : null;`,
      ),
    ).toHaveLength(1);
    expect(
      directionMoveBranches(
        "planted.tsx",
        `const line = summary.direction === "inbound" ? t("email.row.receivedFrom") : t("email.row.sentTo");`,
      ),
    ).toEqual([]);
  });
});
