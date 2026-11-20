import ts from "typescript";
import { describe, expect, it } from "vitest";
import { stable } from "../format/collate";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { vi } from "../i18n/vi";
import {
  enumCodes,
  recordTypesInHistory,
  schemaSource,
  typeMembers,
} from "./contractschema.testkit";
import { historyValueFields, historyValueLabel } from "./historyvaluelabels";

// The coverage census for History's values: every closed set of codes a
// history row can hold has words, in every language.
//
// A code with no words is printed as stored, so "former_customer" reaches the
// reader. The corpus is derived from the generated contract. It holds the
// columns of each record type History serves, the fields their updates write,
// a partner's, and a lead's qualification trigger.

// The schemas of a record type are named by the contract's own entity-type
// enum: the entity itself and its `Update<Type>Request`. A type History gains
// enters the census by itself, and a type with no schema fails it.
//
// A partner extends a company, and a qualification request is a lead's.
// Neither carries its type's name, so they are listed here. The census fails
// when one stops existing.
const EXTENSIONS: ReadonlyMap<string, readonly string[]> = new Map([
  ["company", ["Partner"]],
  ["lead", ["PromoteLeadRequest"]],
]);

// Fields History words by another route, so no second table is demanded.
// `industry` goes through industryWords. A size band is a number range that
// reads as it is stored. `stage_meaning` is read from the stage a partner
// stands on and is never a column, so no history row holds it.
const WORDED_ELSEWHERE = new Set(["industry", "size_band", "stage_meaning"]);

function isReadOnly(member: ts.PropertySignature): boolean {
  return (
    member.modifiers?.some((m) => m.kind === ts.SyntaxKind.ReadonlyKeyword) ??
    false
  );
}

function ownersOf(recordType: string): string[] {
  const entity = `${recordType[0].toUpperCase()}${recordType.slice(1)}`;
  return [
    entity,
    `Update${entity}Request`,
    ...(EXTENSIONS.get(recordType) ?? []),
  ];
}

// Each record type's closed-code fields with their codes.
function closedFieldsByType(): Map<string, Map<string, Set<string>>> {
  const source = schemaSource();
  const byType = new Map<string, Map<string, Set<string>>>();
  for (const recordType of recordTypesInHistory(source)) {
    const fields = new Map<string, Set<string>>();
    for (const owner of ownersOf(recordType)) {
      const members = typeMembers(source, owner);
      expect(members.length, `${owner} for ${recordType}`).toBeGreaterThan(0);
      const entityColumns = !owner.endsWith("Request");
      for (const member of members) {
        if (!ts.isIdentifier(member.name)) continue;
        // A read-only property is computed on read, so no write stores it.
        if (entityColumns && isReadOnly(member)) continue;
        const codes = enumCodes(source, member);
        if (!codes) continue;
        const field = member.name.text;
        fields.set(field, new Set([...(fields.get(field) ?? []), ...codes]));
      }
    }
    for (const field of WORDED_ELSEWHERE) fields.delete(field);
    byType.set(recordType, fields);
  }
  return byType;
}

function closedFields(): Map<string, Set<string>> {
  const merged = new Map<string, Set<string>>();
  for (const fields of closedFieldsByType().values()) {
    for (const [field, codes] of fields) {
      merged.set(field, new Set([...(merged.get(field) ?? []), ...codes]));
    }
  }
  return merged;
}

const LOCALES = [
  ["en", en],
  ["de", de],
  ["vi", vi],
] as const;

describe("history value labels", () => {
  // A narrowed walk reads a smaller contract, finds nothing to check and
  // passes, so the corpus is proved to be a real one first.
  it("finds the closed-code fields of the contract", () => {
    const fields = closedFields();
    for (const expected of [
      "lifecycle",
      "status",
      "trigger",
      "margin_tier",
      "priority",
      "commercial_motion",
      "meeting_status",
    ]) {
      expect([...fields.keys()], expected).toContain(expected);
    }
    expect(fields.get("lifecycle")).toContain("former_customer");
    expect(fields.get("margin_tier")).toContain("tier2_20");
    expect(fields.get("trigger")).toContain("human_qualify");
  });

  it("has words in every language for every code of every closed field", () => {
    const missing: string[] = [];
    for (const [field, codes] of closedFields()) {
      for (const code of codes) {
        for (const [locale] of LOCALES) {
          const words = historyValueLabel(field, code, locale);
          if (words === undefined || words === "" || words === code) {
            missing.push(`${field}=${code} (${locale})`);
          }
        }
      }
    }
    expect(missing.sort(stable), "codes History would print as stored").toEqual(
      [],
    );
  });

  // Words are keyed by field name, since a history row carries no record
  // type. That is only sound while one field name means one thing, so two
  // record types may share a field only with disjoint codes.
  it("never gives two record types the same code under one field", () => {
    // Company and contact visibility is one setting with one meaning.
    const sameMeaning = new Set(["visibility"]);
    const seen = new Map<string, string>();
    const clashes: string[] = [];
    for (const [recordType, fields] of closedFieldsByType()) {
      for (const [field, codes] of fields) {
        if (sameMeaning.has(field)) continue;
        for (const code of codes) {
          const key = `${field}=${code}`;
          const first = seen.get(key);
          if (first !== undefined && first !== recordType) {
            clashes.push(`${key} on ${first} and ${recordType}`);
          }
          seen.set(key, recordType);
        }
      }
    }
    expect(clashes).toEqual([]);
  });

  it("words no field the contract does not give a closed set", () => {
    const closed = new Set(closedFields().keys());
    const extra = historyValueFields()
      .filter((field) => !closed.has(field))
      .filter((field) => !field.startsWith("qualifying_event"))
      .filter(
        (field) => !["occurred_precision", "call_outcome"].includes(field),
      )
      .sort(stable);
    expect(
      extra,
      `fields worded with no closed set: ${extra.join(", ")}`,
    ).toEqual([]);
  });
});
