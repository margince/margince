import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import {
  historyFieldLabel,
  historyFieldLabelKey,
  historyFieldLabelled,
} from "./historyfieldlabels";

// The coverage census: every wire name a history entry can show has a word,
// and every word answers a field that exists.
//
// A missing key fails nowhere else — the label function falls back to the
// column with its underscores spaced out, which renders "amount minor" at a
// reader and looks deliberate. So the obligation is DERIVED rather than
// listed. Two derivations meet here, both out of `schema.d.ts`, which is
// generated from the contract: the record types this projection serves are the
// `entity_type` enum on `FieldHistoryEntry`, and the fields an update writes
// are the properties of each type's `Update<Type>Request` body. Add a field
// upstream, forget the label, and this fails by name.
//
// It reads the generated types rather than the six Go stores for the same
// reason the client does: the contract is the seam between the two trees.

const schemaPath = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "api",
  "schema.d.ts",
);

function schemaSource(): ts.SourceFile {
  return ts.createSourceFile(
    "schema.d.ts",
    readFileSync(schemaPath, "utf8"),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );
}

// The property of one named type in the contract, wherever it is declared.
function propertyOf(
  source: ts.SourceFile,
  owner: string,
  property: string,
): ts.PropertySignature | undefined {
  let found: ts.PropertySignature | undefined;
  const walk = (node: ts.Node) => {
    if (
      ts.isPropertySignature(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === owner &&
      node.type
    ) {
      for (const member of membersOf(node.type)) {
        if (ts.isIdentifier(member.name) && member.name.text === property) {
          found = member;
        }
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(source);
  return found;
}

function membersOf(node: ts.TypeNode): ts.PropertySignature[] {
  if (ts.isIntersectionTypeNode(node)) {
    return node.types.flatMap(membersOf);
  }
  if (!ts.isTypeLiteralNode(node)) {
    return [];
  }
  return node.members.filter(ts.isPropertySignature);
}

// The record types a field-history row can be ABOUT — the contract's own
// closed enum, so a type added upstream widens this corpus rather than
// slipping past it.
function recordTypesInHistory(source: ts.SourceFile): string[] {
  const entityType = propertyOf(source, "FieldHistoryEntry", "entity_type");
  const declared = entityType?.type;
  if (!declared || !ts.isUnionTypeNode(declared)) {
    return [];
  }
  return declared.types
    .filter(ts.isLiteralTypeNode)
    .map((member) => member.literal)
    .filter(ts.isStringLiteral)
    .map((literal) => literal.text);
}

function updateBodyName(recordType: string): string {
  return `Update${recordType[0].toUpperCase()}${recordType.slice(1)}Request`;
}

function fieldsARecordUpdateWrites(): Map<string, string[]> {
  const source = schemaSource();
  const byType = new Map<string, string[]>();
  for (const recordType of recordTypesInHistory(source)) {
    const body = updateBodyName(recordType);
    let fields: string[] = [];
    const walk = (node: ts.Node) => {
      if (
        ts.isPropertySignature(node) &&
        ts.isIdentifier(node.name) &&
        node.name.text === body &&
        node.type
      ) {
        fields = membersOf(node.type)
          .map((member) => member.name)
          .filter(ts.isIdentifier)
          .map((name) => name.text);
      }
      ts.forEachChild(node, walk);
    };
    walk(source);
    byType.set(recordType, fields);
  }
  return byType;
}

describe("history field labels", () => {
  // Under-recognition is the one way this gate could break in silence: a
  // narrowed walk reads a smaller contract, finds nothing to check and passes.
  // So the corpus is proved to be a real one before it is used.
  it("finds every record type's update body in the contract", () => {
    const byType = fieldsARecordUpdateWrites();
    expect([...byType.keys()].sort()).toEqual([
      "activity",
      "deal",
      "lead",
      "organization",
      "person",
      "project",
    ]);
    expect(byType.get("deal")).toContain("amount_minor");
    for (const [recordType, fields] of byType) {
      expect(fields.length, recordType).toBeGreaterThan(0);
    }
  });

  it("has a word for every field a record update writes", () => {
    const unnamed = [...fieldsARecordUpdateWrites().values()]
      .flat()
      .filter((field) => historyFieldLabelKey(field) === undefined)
      .sort();
    expect(unnamed, `fields with no label key: ${unnamed.join(", ")}`).toEqual(
      [],
    );
  });

  // The other direction. A word for a field no update writes any more is a
  // string nobody can reach and nobody will delete, and it is also how a
  // renamed column keeps looking covered while the new spelling renders raw.
  it("has no word for a field no record update writes", () => {
    const written = new Set([...fieldsARecordUpdateWrites().values()].flat());
    const orphans = historyFieldLabelled()
      .filter((field) => !written.has(field))
      .sort();
    expect(
      orphans,
      `labels for unwritten fields: ${orphans.join(", ")}`,
    ).toEqual([]);
  });

  it("names a key the catalogs actually carry", () => {
    const missing = historyFieldLabelled()
      .map((field) => historyFieldLabelKey(field))
      .filter((key) => key !== undefined && !(key in en));
    expect(missing).toEqual([]);
  });

  // A workspace's own column has no label in any catalog this build ships —
  // its name lives in the custom-field catalog — so the fallback is what a
  // reader gets, and it must still read as words.
  it("spaces out a column it has no word for", () => {
    expect(historyFieldLabel("cf_renewal_owner", (key) => key)).toBe(
      "cf renewal owner",
    );
  });
});
