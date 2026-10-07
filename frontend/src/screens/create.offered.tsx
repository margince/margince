// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { FieldControl } from "../design-system/atoms";
import { ComboBox } from "../design-system/combobox";
import {
  type SearchResult,
  useDebouncedSearch,
} from "../design-system/debouncedsearch";
import type { MessageKey } from "../i18n/en";

/**
 * One record a box offers. `fills` are other fields of the form the record
 * already knows; a pick writes them into whichever of those are still blank,
 * so it never overwrites what the reader typed.
 */
export type NameOffer = SearchResult &
  Readonly<{ hint?: string; fills?: Readonly<Record<string, string>> }>;

// A name box that also offers the records already carrying a name like it.
// Typing names a new record; picking an offer binds that record's id under
// `pickedKey` as well, so the form sends the choice rather than a name the
// server would have to guess from. With no `newHint`, a typed name says nothing.
export type NameOffers = Readonly<{
  search: (query: string) => Promise<readonly NameOffer[]>;
  pickedKey: string;
  pickedHint: MessageKey;
  newHint?: MessageKey;
}>;

/** What the box beneath the field says the save will do with its name. */
export function offeredHint(
  offers: NameOffers,
  fieldKey: string,
  values: Record<string, string>,
  t: (key: MessageKey) => string,
): string | undefined {
  if (values[offers.pickedKey]) {
    return t(offers.pickedHint);
  }
  return values[fieldKey]?.trim() && offers.newHint
    ? t(offers.newHint)
    : undefined;
}

export function OfferedNameControl({
  fieldKey,
  offers,
  control,
  values,
  setValues,
  type,
}: Readonly<{
  fieldKey: string;
  offers: NameOffers;
  control: FieldControl;
  values: Record<string, string>;
  setValues: (next: Record<string, string>) => void;
  type?: "email";
}>) {
  const name = values[fieldKey] ?? "";
  const { results } = useDebouncedSearch(offers.search, name.trim());
  return (
    <ComboBox
      {...control}
      type={type}
      value={name}
      // Each offer's value is the record id, so two records sharing a name
      // stay two distinct picks; the label is what lands in the box.
      suggestions={results}
      onChange={(next) => {
        const picked = results.find((result) => result.value === next);
        const memo = `${offers.pickedKey}:filled`;
        // Any keystroke after a pick is a new name, so the id goes with it.
        setValues({
          ...values,
          ...(picked ? refilled(values, picked.fills, values[memo]) : {}),
          [fieldKey]: picked ? picked.label : next,
          [offers.pickedKey]: picked ? picked.value : "",
          ...(picked ? { [memo]: JSON.stringify(picked.fills ?? {}) } : {}),
        });
      }}
    />
  );
}

/**
 * The picked record's values for the fields it may write: the blank ones, and
 * the ones an EARLIER pick filled and nobody has typed over since. A second
 * pick replaces the first person rather than leaving half of them behind;
 * what the reader typed is never touched.
 */
function refilled(
  values: Record<string, string>,
  fills: Readonly<Record<string, string>> | undefined,
  earlierJSON: string | undefined,
): Record<string, string> {
  const earlier = parsedFills(earlierJSON);
  const out: Record<string, string> = {};
  for (const key of new Set([
    ...Object.keys(earlier),
    ...Object.keys(fills ?? {}),
  ])) {
    const current = values[key] ?? "";
    if (!current.trim() || current === earlier[key]) {
      out[key] = fills?.[key] ?? "";
    }
  }
  return out;
}

function parsedFills(json: string | undefined): Record<string, string> {
  if (!json) return {};
  const parsed: unknown = JSON.parse(json);
  if (!parsed || typeof parsed !== "object") return {};
  return Object.fromEntries(
    Object.entries(parsed).filter(
      (entry): entry is [string, string] => typeof entry[1] === "string",
    ),
  );
}
