// Pure helpers for custom fields, with no React. The admin screen's derivations
// (immutable API key, pending DDL preview, structural-word refusal) are tested
// here in isolation.

import type { components } from "../api/schema";
import { translate } from "../i18n";
import { DEFAULT_LOCALE, isLocale } from "../i18n/locale";
import { translatedLabel } from "./labeltranslation";

export type CfObject = components["schemas"]["CustomField"]["object"];

// Chip order is normative (AC-custom-fields-2), and the SET is the contract's:
// CfObject is the generated `CustomField["object"]`, so this list cannot quietly
// fall behind the objects the engine accepts. That is not hypothetical — it did.
// The four below shipped while the engine had already grown to six, and a field
// an admin could create on a project or a contract was one this screen refused
// to name, with no way to reach it but the API.
// The parity test in customfields.logic.test.ts is what holds the two equal.
export const CF_OBJECTS: readonly CfObject[] = [
  "deal",
  "company",
  "contact",
  "lead",
  "project",
  "contract",
  "activity",
];

/** A record key's custom-field catalog, where that record type has one. */
export function catalogOfRecord(recordKey: string): CfObject | undefined {
  return CF_OBJECTS.find((object) => object === recordKey);
}

/** A field's name in the reader's language, without any retired mark. */
export function customFieldName(
  field: components["schemas"]["CustomField"],
  locale: string,
): string {
  return translatedLabel(field.label, field.label_translations, locale);
}

/**
 * A field's label as the reader sees it. A retired field reads "Label (retired)",
 * because a live field may carry the same name. This is the one place that adds it.
 */
export function customFieldLabel(
  field: components["schemas"]["CustomField"],
  locale: string,
): string {
  const label = customFieldName(field, locale);
  if (field.status !== "retired") {
    return label;
  }
  return translate(
    isLocale(locale) ? locale : DEFAULT_LOCALE,
    "cf.retiredLabel",
    {
      label,
    },
  );
}

// The type set mirrors fieldcatalog.Types() in the backend, in its order.
// backend/gates/frontendvocabularymirrors_test.go fails when the two differ in
// either direction. The create request takes a CfType, so a member the
// contract lacks fails to compile.
export const CF_TYPES = [
  "text",
  "number",
  "date",
  "currency",
  "picklist",
  "multiselect",
  "boolean",
  "record",
  "url",
] as const;

export type CfType = (typeof CF_TYPES)[number];

export function slug(label: string): string {
  return label
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_|_$/g, "");
}

export function columnName(label: string): string {
  return `cf_${slug(label) || "…"}`;
}

export function apiKey(object: CfObject, label: string): string {
  return `${object}.${columnName(label)}`;
}

// A field is data; an object or a relationship is structure. A label that reads
// like a structural request is refused pre-emptively (AC-custom-fields-5).
//
// Single, bare structural nouns match on WORD BOUNDARIES so an innocent label
// that merely contains one as a substring ("Objective score" has "object",
// "Notable accounts" / "Portable device" have "table") is not refused. The
// multi-word relationship phrases are distinctive enough to match as substrings.
// Word-boundary alternatives, singular and plural (entity → entities), so
// "New objects" / "Related entities" are refused, not just their singular.
const STRUCTURE_TOKENS: readonly string[] = [
  "objects?",
  "tables?",
  "entit(?:y|ies)",
];

const STRUCTURE_PHRASES: readonly string[] = [
  "relationship",
  "link to",
  "related to",
  "lookup to",
  "connect to",
  "associate with",
  "child of",
  "parent of",
  "many-to-many",
  "one-to-many",
];

const STRUCTURE_TOKEN_RE = new RegExp(
  `\\b(?:${STRUCTURE_TOKENS.join("|")})\\b`,
);

export function looksStructural(label: string): boolean {
  const l = label.toLowerCase();
  if (STRUCTURE_TOKEN_RE.test(l)) {
    return true;
  }
  return STRUCTURE_PHRASES.some((phrase) => l.includes(phrase));
}
