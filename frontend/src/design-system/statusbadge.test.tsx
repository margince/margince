/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen } from "@testing-library/react";
import ts from "typescript";
import { afterEach, describe, expect, it } from "vitest";
import {
  filesUnder,
  moduleSpecifiers,
  parseSource,
  sourceFileAt,
} from "../../scripts/lib/source-tree";
import { STATUS_KINDS, StatusBadge, statusTone } from "./statusbadge";

afterEach(cleanup);

function badgeFor(label: string): HTMLElement {
  const badge = screen.getByText(label).closest(".badge");
  if (!(badge instanceof HTMLElement)) {
    throw new Error(`no .badge carries "${label}"`);
  }
  return badge;
}

describe("StatusBadge", () => {
  it("maps each kind to the one tone the palette gives it", () => {
    expect(STATUS_KINDS.map(statusTone)).toEqual([
      "success",
      "default",
      "warning",
      "danger",
    ]);
  });

  it("draws neutral as the plain grey badge", () => {
    render(<StatusBadge kind="neutral" label="Lost" />);
    expect(badgeFor("Lost").className).toBe("badge");
  });

  it("draws each other kind in its tone", () => {
    render(<StatusBadge kind="failure" label="Failed" />);
    expect(badgeFor("Failed")).toHaveClass("badge-danger");
  });

  it("puts the live dot first when the status is running", () => {
    render(<StatusBadge kind="neutral" label="Syncing" live />);
    const first = badgeFor("Syncing").firstElementChild;
    expect(first).toHaveClass("badge-live-dot");
  });

  it("draws no dot when the status is not live", () => {
    render(<StatusBadge kind="done" label="Connected" />);
    expect(badgeFor("Connected").querySelector(".badge-live-dot")).toBeNull();
  });
});

// Blue never means status. A module that uses this palette may not keep a
// second map beside it that still hands out a blue tone.
const BLUE = new Set(["info", "accent"]);
const STATUS_TONES = new Set(["success", "warning", "danger", "default"]);

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const sourceRoot = join(frontendRoot, "src");

function literalOf(node: ts.Node | undefined): string | null {
  return node && ts.isStringLiteralLike(node) ? node.text : null;
}

function namedTone(name: ts.Node): boolean {
  return (
    (ts.isIdentifier(name) || ts.isStringLiteral(name)) && name.text === "tone"
  );
}

/** A tone union or tone map typed out by hand, with blue among the statuses. */
function blueAmongStatuses(values: (string | null)[]): boolean {
  const known = values.filter((value): value is string => value !== null);
  return (
    known.some((value) => BLUE.has(value)) &&
    known.some((value) => STATUS_TONES.has(value))
  );
}

function blueToneAt(node: ts.Node): boolean {
  // A Callout or Panel may still be blue; only a Badge's tone is a status.
  if (ts.isJsxAttribute(node) && namedTone(node.name)) {
    const element = node.parent.parent;
    return (
      element.tagName.getText() === "Badge" &&
      BLUE.has(literalOf(node.initializer) ?? "")
    );
  }
  if (ts.isPropertyAssignment(node) && namedTone(node.name)) {
    return BLUE.has(literalOf(node.initializer) ?? "");
  }
  if (ts.isUnionTypeNode(node)) {
    return blueAmongStatuses(
      node.types.map((member) =>
        ts.isLiteralTypeNode(member) ? literalOf(member.literal) : null,
      ),
    );
  }
  if (ts.isObjectLiteralExpression(node)) {
    return blueAmongStatuses(
      node.properties.map((property) =>
        ts.isPropertyAssignment(property)
          ? literalOf(property.initializer)
          : null,
      ),
    );
  }
  return false;
}

function blueStatusLines(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node): void => {
    if (blueToneAt(node)) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart()).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return lines;
}

function usesPalette(source: ts.SourceFile): boolean {
  return moduleSpecifiers(source, "all").some((specifier) =>
    /(^|\/)statusbadge$/.test(specifier),
  );
}

describe("status palette fitness", () => {
  const corpus = filesUnder(sourceRoot);
  const users = corpus.filter((path) => usesPalette(sourceFileAt(path)));

  it("reads the whole tree and finds the palette in use", () => {
    expect(corpus.length).toBeGreaterThan(500);
    expect(users.map((path) => relative(sourceRoot, path))).toContain(
      "design-system/statusbadge.stories.tsx",
    );
  });

  it("finds no blue status tone in a module that uses the palette", () => {
    const findings = users.flatMap((path) =>
      blueStatusLines(sourceFileAt(path)).map(
        (line) => `${relative(sourceRoot, path)}:${line}`,
      ),
    );
    expect(findings).toEqual([]);
  });

  it.each([
    ["a JSX tone", `<Badge tone="info">x</Badge>`],
    ["a tone property", `const a = { tone: "accent", label: "x" };`],
    ["a tone union", `type T = "info" | "success" | undefined;`],
    ["a status map", `const M = { invited: "info", active: "success" };`],
  ])("catches %s", (_shape, code) => {
    expect(blueStatusLines(parseSource("planted.tsx", code))).toHaveLength(1);
  });

  it("leaves a blue Callout alone", () => {
    const code = `<Callout tone="info">x</Callout>`;
    expect(blueStatusLines(parseSource("planted.tsx", code))).toEqual([]);
  });

  it("leaves a map with no blue in it alone", () => {
    const code = `const M = { invited: "default", active: "success" };`;
    expect(blueStatusLines(parseSource("planted.tsx", code))).toEqual([]);
  });
});
