// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Search } from "lucide-react";
import { useCallback, useRef, useState } from "react";
import { holdsEveryWord, searchWords } from "../format/collate";
import { translate, useLocale, useT } from "../i18n";
import { useDebouncedSearch } from "./debouncedsearch";
import type { ListChip } from "./listsurface.dials";
import { SuggestPopup, useSuggestList } from "./suggestlist";

type FilterOffer = Readonly<{
  value: string;
  label: string;
  chip: string;
  filterValue: string;
  filterLabel: string;
  term: string;
}>;

export function ListSearch({
  search,
  chips,
  chosen,
  onPick,
  onFocus,
  menuOpen,
}: Readonly<{
  search: { value: string; onChange: (next: string) => void };
  chips: readonly ListChip[];
  chosen: Readonly<Record<string, string>>;
  onPick?: (key: string, value: string, label: string) => void;
  onFocus: () => void;
  menuOpen: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const input = useRef<HTMLInputElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const [focused, setFocused] = useState(false);
  const term = search.value.trim();
  const canFilter = chips.length > 0 && onPick !== undefined;
  const sources = chips.filter((chip) => chip.search);
  const [retainedSources, setRetainedSources] = useState(sources);
  if (
    sources.length !== retainedSources.length ||
    sources.some((chip, index) => {
      const before = retainedSources[index];
      return (
        !before ||
        chip.key !== before.key ||
        chip.label !== before.label ||
        chip.search !== before.search
      );
    })
  ) {
    setRetainedSources(sources);
  }
  const offer = useCallback(
    (
      chip: ListChip,
      value: string,
      label: string,
      query: string,
    ): FilterOffer => ({
      value: JSON.stringify([chip.key, value]),
      label: `${chip.label} ${translate(locale, "table.filterIs")} ${label}`,
      chip: chip.key,
      filterValue: value,
      filterLabel: label,
      term: query,
    }),
    [locale],
  );
  const find = useCallback(
    async (query: string) =>
      (
        await Promise.all(
          retainedSources.map(async (chip) => {
            if (!chip.search) return [];
            const labelWords = searchWords(chip.label);
            const prefix = (query.match(/\S+\s*/gu) ?? [])
              .slice(0, labelWords.length)
              .join("");
            const valueQuery =
              searchWords(prefix).join(" ") === labelWords.join(" ")
                ? query.slice(prefix.length).trim()
                : query;
            const found = await chip.search(valueQuery);
            return found.map((row) => offer(chip, row.value, row.label, query));
          }),
        )
      ).flat(),
    [retainedSources, offer],
  );
  const remote = useDebouncedSearch(
    focused && canFilter ? find : undefined,
    term,
  );
  const words = searchWords(term);
  const fixed = chips.flatMap((chip) =>
    chip.search
      ? []
      : chip.options
          .filter(
            (row) => term && holdsEveryWord(words, [chip.label, row.label]),
          )
          .map((row) => offer(chip, row.value, row.label, term)),
  );
  const offers = [
    ...fixed,
    ...remote.results.filter((row) => row.term === term),
  ]
    .filter((row) => chosen[row.chip] !== row.filterValue)
    .slice(0, 20);
  const suggestions =
    canFilter && term
      ? [
          ...offers,
          {
            value: "text-search",
            label: t("list.searchFor", { query: search.value }),
            hint: remote.failed
              ? t("list.filterSuggestionsFailed")
              : remote.pending
                ? t("table.filterSearching")
                : undefined,
          },
        ]
      : [];
  const list = useSuggestList({
    anchorRef: input,
    popupRef: popup,
    suggestions,
    typed: search.value,
    disabled: menuOpen || !canFilter,
    onPick: (value) => {
      const picked = offers.find((row) => row.value === value);
      if (picked) {
        onPick?.(picked.chip, picked.filterValue, picked.filterLabel);
        search.onChange("");
      }
      input.current?.focus();
    },
  });
  return (
    <>
      <label className="lt-search">
        <Search size={13} strokeWidth={1.6} aria-hidden="true" />
        <span className="sr-only">{t("list.search")}</span>
        <input
          ref={input}
          className="lt-search-input"
          {...(canFilter ? list.fieldAria : {})}
          type="search"
          value={search.value}
          placeholder={t("list.search")}
          autoComplete="off"
          onChange={(event) => {
            search.onChange(event.target.value);
            list.retype();
          }}
          onFocus={() => {
            onFocus();
            setFocused(true);
            list.show();
          }}
          onBlur={() => {
            setFocused(false);
            list.close();
          }}
          onKeyDown={(event) => {
            if (canFilter && !list.navigate(event) && event.key === "Enter")
              list.close();
          }}
        />
      </label>
      <SuggestPopup list={list} />
    </>
  );
}
