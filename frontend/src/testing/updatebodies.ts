// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What each record type's update body writes, read out of `schema.d.ts`, which
// is generated from the contract. One reader for every gate that needs it.

import ts from "typescript";
import {
  recordTypesInHistory,
  schemaSource,
  typeMembers,
} from "../lib/contractschema.testkit";

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
    const members = typeMembers(source, updateBodyName(recordType)).filter(
      (member) =>
        ts.isIdentifier(member.name) && !INSTRUCTIONS.has(member.name.text),
    );
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
