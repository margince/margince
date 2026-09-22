// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { useT } from "../i18n";
import { Radio } from "./atoms";
import { useDebouncedSearch } from "./debouncedsearch";
import type { ListChip } from "./listsurface.dials";

// THE VALUE STEP of a filter, in the two shapes an attribute can take: a fixed
// list of options, or a search over a set too large to draw whole. Both end at
// one value for one attribute, which is why the menu that opens them does not
// care which it got.

/**
 * The "all" entry plus every option, shared by the Filter button's value
 * step and an applied chip's own reopened menu — the same list either way,
 * since both end at picking one value for one attribute.
 */
function FilterValueList({
  chip,
  value,
  onPick,
}: Readonly<{
  chip: ListChip;
  value: string;
  onPick: (value: string, label: string) => void;
}>) {
  // One filter takes one value, so these are radios; the name groups them and
  // the enclosing `Menu` fieldset is what names the question they answer.
  const group = useId();
  return (
    <>
      <Radio
        className="lt-mi"
        name={group}
        checked={!value}
        label={chip.allLabel}
        onChange={() => onPick("", chip.allLabel)}
      />
      {chip.options.map((option) => (
        <Radio
          key={option.value}
          className="lt-mi"
          name={group}
          value={option.value}
          checked={option.value === value}
          label={option.label}
          onChange={() => onPick(option.value, option.label)}
        />
      ))}
    </>
  );
}

/**
 * A relation filter's value step when the attribute is too large to list
 * whole: a text box in place of the fixed options, searching on debounce.
 * The four honest states a search can be in — nothing typed, in flight, no
 * matches, failed — are each their own line, and a query in flight keeps the
 * previous results on screen rather than clearing the menu out from under
 * the reader while the next answer is still coming back.
 */
function AsyncFilterValueList({
  chip,
  value,
  onPick,
}: Readonly<{
  chip: ListChip;
  value: string;
  onPick: (value: string, label: string) => void;
}>) {
  const t = useT();
  const [query, setQuery] = useState("");
  const search = chip.search;
  const group = useId();
  const { results, pending, failed } = useDebouncedSearch(search, query);

  if (!search) {
    return null;
  }

  return (
    <>
      <Radio
        className="lt-mi"
        name={group}
        checked={!value}
        label={chip.allLabel}
        onChange={() => onPick("", chip.allLabel)}
      />
      <label className="lt-fsearch">
        <span className="sr-only">
          {t("table.filterValueSearch", { filter: chip.label })}
        </span>
        <input
          className="lt-fsearch-input"
          value={query}
          placeholder={t("table.filterValueSearch", { filter: chip.label })}
          onChange={(event) => setQuery(event.target.value)}
        />
      </label>
      {!query && (
        <p className="lt-fvalue-status">{t("table.filterTypeToSearch")}</p>
      )}
      {query && pending && (
        <p className="lt-fvalue-status">{t("table.filterSearching")}</p>
      )}
      {query && failed && (
        <p className="lt-fvalue-status error">
          {t("table.filterSearchFailed")}
        </p>
      )}
      {query && !pending && !failed && results.length === 0 && (
        <p className="lt-fvalue-status">{t("table.filterNoMatches")}</p>
      )}
      {results.map((option) => (
        <Radio
          key={option.value}
          className="lt-mi"
          name={group}
          value={option.value}
          checked={option.value === value}
          label={option.label}
          onChange={() => onPick(option.value, option.label)}
        />
      ))}
    </>
  );
}

/** Either value step a filter row's value segment can open, keyed by whether
 * the chip declares a search source. */
export function ChipValueList({
  chip,
  value,
  onPick,
}: Readonly<{
  chip: ListChip;
  value: string;
  onPick: (value: string, label: string) => void;
}>) {
  return chip.search ? (
    <AsyncFilterValueList chip={chip} value={value} onPick={onPick} />
  ) : (
    <FilterValueList chip={chip} value={value} onPick={onPick} />
  );
}
