// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { FieldControl } from "../design-system/atoms";
import { ComboBox } from "../design-system/combobox";
import {
  type SearchResult,
  useDebouncedSearch,
} from "../design-system/debouncedsearch";
import type { MessageKey } from "../i18n/en";

// A name box that also offers the records already carrying a name like it.
// Typing names a new record; picking an offer binds that record's id under
// `pickedKey` as well, so the form sends the choice rather than a name the
// server would have to guess from.
export type NameOffers = Readonly<{
  search: (query: string) => Promise<readonly SearchResult[]>;
  pickedKey: string;
  pickedHint: MessageKey;
  newHint: MessageKey;
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
  return values[fieldKey]?.trim() ? t(offers.newHint) : undefined;
}

export function OfferedNameControl({
  fieldKey,
  offers,
  control,
  values,
  setValues,
}: Readonly<{
  fieldKey: string;
  offers: NameOffers;
  control: FieldControl;
  values: Record<string, string>;
  setValues: (next: Record<string, string>) => void;
}>) {
  const name = values[fieldKey] ?? "";
  const { results } = useDebouncedSearch(offers.search, name.trim());
  return (
    <ComboBox
      {...control}
      value={name}
      // Each offer's value is the record id, so two records sharing a name
      // stay two distinct picks; the label is what lands in the box.
      suggestions={results}
      onChange={(next) => {
        const picked = results.find((result) => result.value === next);
        // Any keystroke after a pick is a new name, so the id goes with it.
        setValues({
          ...values,
          [fieldKey]: picked ? picked.label : next,
          [offers.pickedKey]: picked ? picked.value : "",
        });
      }}
    />
  );
}
