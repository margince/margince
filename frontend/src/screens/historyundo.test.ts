import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { undoRefusalKey, undoRefusalsNamed } from "./historyundo";

// One refusal set, two spellings on this side of the wire: the contract's enum
// and the words a reader gets. A reason the server can return and this build
// has no sentence for renders as a refused button with nothing on it, which is
// the exact defect the feature exists to remove — so the set is DERIVED from
// the generated contract rather than restated beside the map it checks.

const schemaPath = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "api",
  "schema.d.ts",
);

// The `reason` members `Undoability` declares. Read off the generated types,
// which openapi-typescript writes as a union of string literals plus null.
function refusalsInContract(): string[] {
  const source = ts.createSourceFile(
    "schema.d.ts",
    readFileSync(schemaPath, "utf8"),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );
  let members: string[] = [];
  const walk = (node: ts.Node) => {
    if (
      ts.isPropertySignature(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "Undoability" &&
      node.type &&
      ts.isTypeLiteralNode(node.type)
    ) {
      for (const member of node.type.members) {
        if (
          ts.isPropertySignature(member) &&
          ts.isIdentifier(member.name) &&
          member.name.text === "reason" &&
          member.type &&
          ts.isUnionTypeNode(member.type)
        ) {
          members = member.type.types
            .filter(ts.isLiteralTypeNode)
            .map((literal) => literal.literal)
            .filter(ts.isStringLiteral)
            .map((literal) => literal.text);
        }
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(source);
  return members;
}

describe("undo refusals", () => {
  // Under-recognition is the one way this gate could break in silence: a walk
  // that finds nothing compares two empty sets and passes.
  it("finds the refusals the contract declares", () => {
    expect(refusalsInContract()).toContain("superseded");
    expect(refusalsInContract().length).toBeGreaterThanOrEqual(10);
  });

  it("has a sentence for every refusal the server can return", () => {
    const wordless = refusalsInContract()
      .filter((reason) => undoRefusalKey(reason) === undefined)
      .sort();
    expect(
      wordless,
      `refusals with no sentence: ${wordless.join(", ")}`,
    ).toEqual([]);
  });

  // The other direction: a sentence for a refusal the contract no longer names
  // is copy nobody can reach, and it is how a renamed reason keeps looking
  // covered while the new spelling arrives wordless.
  it("has no sentence for a refusal the contract does not name", () => {
    const declared = new Set(refusalsInContract());
    const orphans = undoRefusalsNamed()
      .filter((reason) => !declared.has(reason))
      .sort();
    expect(
      orphans,
      `sentences for unnamed refusals: ${orphans.join(", ")}`,
    ).toEqual([]);
  });

  it("names keys the catalogs actually carry", () => {
    const missing = undoRefusalsNamed()
      .map((reason) => undoRefusalKey(reason))
      .filter((key) => key !== undefined && !(key in en));
    expect(missing).toEqual([]);
  });

  // A reason this build predates has no sentence, and inventing one would
  // describe a case nobody has written down. The caller falls back to what the
  // server said instead.
  it("has no words for a refusal it has never heard of", () => {
    expect(undoRefusalKey("invented_upstream")).toBeUndefined();
    expect(undoRefusalKey(null)).toBeUndefined();
  });
});
