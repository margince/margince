// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type FieldLabels,
  fieldCheckSentence,
  problemCodeOf,
  type RefusalSentences,
  refusalSentences,
} from "../../lib/problem";

// The editor's fields, flat. The contract nests the address and the offering;
// draftOf and changesOf map between the two shapes.
export type Draft = Readonly<{
  display_name: string;
  legal_name: string;
  line1: string;
  line2: string;
  postal_code: string;
  city: string;
  region: string;
  country: string;
  jurisdiction: string;
  vat_id: string;
  register_court: string;
  register_number: string;
  default_language: string;
  bank_account_holder: string;
  iban: string;
  bic: string;
  imprint: string;
  signature_template: string;
  offer_summary: string;
  icp: string;
  value_proposition: string;
  usp: string;
}>;
export type DraftKey = keyof Draft;

export const ADDRESS_KEYS = [
  "line1",
  "line2",
  "postal_code",
  "city",
  "region",
] as const satisfies readonly DraftKey[];
export const OFFERING_KEYS = [
  "offer_summary",
  "icp",
  "value_proposition",
  "usp",
] as const satisfies readonly DraftKey[];
export const FLAT_KEYS = [
  "display_name",
  "legal_name",
  "country",
  "jurisdiction",
  "vat_id",
  "register_court",
  "register_number",
  "default_language",
  "bank_account_holder",
  "iban",
  "bic",
  "imprint",
  "signature_template",
] as const satisfies readonly DraftKey[];

export const LABEL_KEYS: Readonly<Record<DraftKey, MessageKey>> = {
  display_name: "legalEntities.displayName",
  legal_name: "legalEntities.legalName",
  line1: "legalEntities.line1",
  line2: "legalEntities.line2",
  postal_code: "legalEntities.postalCode",
  city: "legalEntities.city",
  region: "legalEntities.region",
  country: "legalEntities.country",
  jurisdiction: "legalEntities.jurisdiction",
  vat_id: "legalEntities.vatId",
  register_court: "legalEntities.registerCourt",
  register_number: "legalEntities.registerNumber",
  default_language: "legalEntities.defaultLanguage",
  bank_account_holder: "legalEntities.bankHolder",
  iban: "legalEntities.iban",
  bic: "legalEntities.bic",
  imprint: "legalEntities.imprint",
  signature_template: "legalEntities.signatureTemplate",
  offer_summary: "legalEntities.offerSummary",
  icp: "legalEntities.icp",
  value_proposition: "legalEntities.valueProposition",
  usp: "legalEntities.usp",
};

// What the default entity keeps itself; the rest it reads from the company.
export const DEFAULT_ENTITY_KEYS = [
  "bank_account_holder",
  "iban",
  "bic",
  "default_language",
  "imprint",
] as const satisfies readonly DraftKey[];

// The fields every further entity must have.
const REQUIRED_ENTITY_FIELDS = [
  "display_name",
  "legal_name",
  "country",
  "jurisdiction",
] as const satisfies readonly DraftKey[];

const NAME_TAKEN = "legal_entity_name_taken";

const ADDRESS_KEY_SET: ReadonlySet<DraftKey> = new Set<DraftKey>(ADDRESS_KEYS);
const OFFERING_KEY_SET: ReadonlySet<DraftKey> = new Set<DraftKey>(
  OFFERING_KEYS,
);
const OFFERING_PREFIX = "offering.";

/** Every field the editor holds. */
export const ENTITY_KEYS: readonly DraftKey[] = [
  ...FLAT_KEYS,
  ...ADDRESS_KEYS,
  ...OFFERING_KEYS,
];

// The contract nests the address, so a refusal names `address.city`.
export function wireOf(key: DraftKey): string {
  return ADDRESS_KEY_SET.has(key) ? `address.${key}` : key;
}

/**
 * The on-screen label of each field the form shows, by its wire path. The
 * request decoder names an offering field `offering.icp`, the store `icp`.
 */
export function entityLabels(
  t: Translator,
  keys: readonly DraftKey[],
): FieldLabels {
  return Object.fromEntries(
    keys.flatMap((key) => {
      const label = t(LABEL_KEYS[key]);
      const named: [string, string][] = [[wireOf(key), label]];
      if (OFFERING_KEY_SET.has(key)) {
        named.push([`${OFFERING_PREFIX}${key}`, label]);
      }
      return named;
    }),
  );
}

/**
 * A refused entity write split between its fields and the foot. A taken name
 * answers 409 with its own code and no field, and belongs on the name.
 */
export function entityRefusal(
  error: unknown,
  t: Translator,
  labels: FieldLabels,
): RefusalSentences {
  if (problemCodeOf(error) === NAME_TAKEN) {
    return {
      byField: new Map([["display_name", t("legalEntities.nameTaken")]]),
      rest: null,
    };
  }
  const refused = refusalSentences(error, t, labels);
  const byField = new Map<string, string>();
  for (const [wire, sentence] of refused.byField) {
    const field = wire.startsWith(OFFERING_PREFIX)
      ? wire.slice(OFFERING_PREFIX.length)
      : wire;
    if (!byField.has(field)) byField.set(field, sentence);
  }
  return { byField, rest: refused.rest };
}

/** The sentence for each required field left blank, before anything is sent. */
export function missingEntityFields(
  values: Readonly<Partial<Record<DraftKey, string | null | undefined>>>,
  labels: FieldLabels,
  locale: Locale,
): ReadonlyMap<string, string> {
  return new Map(
    REQUIRED_ENTITY_FIELDS.filter(
      (key) => (values[key] ?? "").trim() === "",
    ).map((key) => [key, fieldCheckSentence("required", locale, labels[key])]),
  );
}
