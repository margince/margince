import ts from "typescript";
import { describe, expect, it } from "vitest";
import { stable } from "../format/collate";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { vi } from "../i18n/vi";
import { enumCodes, schemaSource, typeMembers } from "./contractschema.testkit";
import { historyValueFields, historyValueLabel } from "./historyvaluelabels";
import { SHIPPED_LABEL_KEYS } from "./shippedlabels";

// The coverage census for History's values: every closed set of codes a
// history row can hold has words, in every language.
//
// A code with no words is printed as stored, so "former_customer" reaches the
// reader looking deliberate. The corpus is derived from the generated contract:
// the columns of each record type History serves, the fields their updates
// write, a partner's, and a lead's qualification trigger.

const RECORD_TYPES = [
  "Activity",
  "Company",
  "Contact",
  "Deal",
  "Lead",
  "Project",
  "Partner",
];
const WRITE_BODIES = [
  "UpdateActivityRequest",
  "UpdateCompanyRequest",
  "UpdateContactRequest",
  "UpdateDealRequest",
  "UpdateLeadRequest",
  "UpdateProjectRequest",
  "PromoteLeadRequest",
];

// Fields History words by another route, which this census must not demand a
// second table for: `industry` goes through industryWords, and a size band is
// a number range that reads as it is stored.
// `stage_meaning` is read from the stage a partner stands on and is never a
// column of the partner, so no history row holds it.
const WORDED_ELSEWHERE = new Set(["industry", "size_band", "stage_meaning"]);

function isReadOnly(member: ts.PropertySignature): boolean {
  return (
    member.modifiers?.some((m) => m.kind === ts.SyntaxKind.ReadonlyKeyword) ??
    false
  );
}

// Each closed-code field the contract gives a stored column, with its codes.
function closedFields(): Map<string, Set<string>> {
  const source = schemaSource();
  const byField = new Map<string, Set<string>>();
  const owners = [...RECORD_TYPES, ...WRITE_BODIES];
  for (const owner of owners) {
    const columnsOnly = RECORD_TYPES.includes(owner);
    for (const member of typeMembers(source, owner)) {
      if (!ts.isIdentifier(member.name)) continue;
      // A read-only property is computed on read, so no write stores it.
      if (columnsOnly && isReadOnly(member)) continue;
      const codes = enumCodes(source, member);
      if (!codes) continue;
      const field = member.name.text;
      byField.set(field, new Set([...(byField.get(field) ?? []), ...codes]));
    }
  }
  for (const field of WORDED_ELSEWHERE) byField.delete(field);
  return byField;
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

  it("words no field the contract does not give a closed set", () => {
    const closed = new Set(closedFields().keys());
    const extra = historyValueFields()
      .filter((field) => !closed.has(field))
      .filter((field) => field !== "relationship_stage")
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

  it("words every stage the product ships", () => {
    const stages = SHIPPED_LABEL_KEYS.filter((key) =>
      key.startsWith("partnerStages.builtin."),
    ).map((key) => key.slice("partnerStages.builtin.".length));
    expect(stages.length).toBeGreaterThan(5);
    for (const stage of stages) {
      for (const [locale, catalog] of LOCALES) {
        const key = `partnerStages.builtin.${stage}`;
        expect(
          historyValueLabel("relationship_stage", stage, locale),
          `${stage} (${locale})`,
        ).toBe(catalog[key as keyof typeof catalog]);
      }
    }
  });

  it("leaves a stage an administrator added as stored", () => {
    expect(historyValueLabel("relationship_stage", "board_review", "en")).toBe(
      "board_review",
    );
  });
});
