// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What each record type's update body writes, read out of `schema.d.ts`, which
// is generated from the contract. One reader for every gate that needs it.

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { sourceFileAt } from "../../scripts/lib/source-tree";

const schemaPath = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "api",
  "schema.d.ts",
);

function schemaSource(): ts.SourceFile {
  return sourceFileAt(schemaPath);
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

// The record types a field-history row can be ABOUT: the contract's own closed enum.
// A type added upstream widens this corpus instead of slipping past it.
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

// Body fields that tell the update what else to do and are never a column it
// writes, so no history row can name them.
// Filing uses a relink audit; a follow-up creates a separately audited task.
const INSTRUCTIONS = new Set([
  "convert_lead_ids",
  "deal_filing",
  "follow_up_task",
]);

function updateBodyName(recordType: string): string {
  return `Update${recordType.charAt(0).toUpperCase()}${recordType.slice(1)}Request`;
}

// Each record type's update body, as the members the contract declares.
function updateBodies(
  source: ts.SourceFile,
): Map<string, ts.PropertySignature[]> {
  const byType = new Map<string, ts.PropertySignature[]>();
  for (const recordType of recordTypesInHistory(source)) {
    const body = updateBodyName(recordType);
    let members: ts.PropertySignature[] = [];
    const walk = (node: ts.Node) => {
      if (
        ts.isPropertySignature(node) &&
        ts.isIdentifier(node.name) &&
        node.name.text === body &&
        node.type
      ) {
        members = membersOf(node.type).filter(
          (member) =>
            ts.isIdentifier(member.name) && !INSTRUCTIONS.has(member.name.text),
        );
      }
      ts.forEachChild(node, walk);
    };
    walk(source);
    byType.set(recordType, members);
  }
  return byType;
}

function nameOf(member: ts.PropertySignature): string {
  return ts.isIdentifier(member.name) ? member.name.text : "";
}

/** The columns each record type's update writes, by record type. */
export function fieldsARecordUpdateWrites(): Map<string, string[]> {
  return new Map(
    [...updateBodies(schemaSource())].map(([recordType, members]) => [
      recordType,
      members.map(nameOf),
    ]),
  );
}

// openapi-typescript writes a `format: date` field's format in the comment
// above it; `date-time` is an instant, not a calendar day.
const CALENDAR_DAY_FORMAT = /\bFormat: date(?!-)\b/;

/** The columns any record update writes as a calendar day. */
export function calendarDayFieldsAnUpdateWrites(): string[] {
  const source = schemaSource();
  const days = new Set<string>();
  for (const members of updateBodies(source).values()) {
    for (const member of members) {
      const comments =
        ts.getLeadingCommentRanges(source.text, member.getFullStart()) ?? [];
      if (
        comments.some((range) =>
          CALENDAR_DAY_FORMAT.test(source.text.slice(range.pos, range.end)),
        )
      ) {
        days.add(nameOf(member));
      }
    }
  }
  return [...days];
}
