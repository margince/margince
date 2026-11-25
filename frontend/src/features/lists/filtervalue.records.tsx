// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The records an id clause already names, and the search that finds a company
// to add: the half of the filter value control that speaks in records rather
// than in scalars.

import { X } from "lucide-react";
import { useLayoutEffect, useRef, useState } from "react";
import { useEntityName } from "../../components/entityref.queries";
import { Button } from "../../design-system/atoms";
import {
  type SearchResult,
  useDebouncedSearch,
} from "../../design-system/debouncedsearch";
import { SuggestPopup, useSuggestList } from "../../design-system/suggestlist";
import { useT } from "../../i18n";
import "./filterbuilder.css";
import { searchCompanies } from "../../components/filterreference";
import type { LeafValue } from "../../lib/segmentpredicate";

/**
 * The ids a list clause already names, and a way to drop one.
 *
 * Shown under the labels the caller holds, else under the company's name,
 * which a clause restored from a saved view needs. The id shows only while
 * that name loads or when the reader may not see the company.
 */
export function ChosenRecords({
  ids,
  labels,
  onRemove,
  lookup,
}: Readonly<{
  ids: readonly string[];
  labels: ReadonlyMap<string, string>;
  onRemove: (id: string) => void;
  /** The kind of record the ids name, when the caller has no labels for them. */
  lookup?: "company";
}>) {
  if (ids.length === 0) {
    return null;
  }
  return (
    <ul className="filter-chosen">
      {ids.map((id) => (
        <ChosenRecord
          key={id}
          id={id}
          label={labels.get(id)}
          lookup={lookup}
          onRemove={onRemove}
        />
      ))}
    </ul>
  );
}

function ChosenRecord({
  id,
  label,
  lookup,
  onRemove,
}: Readonly<{
  id: string;
  label: string | undefined;
  lookup: "company" | undefined;
  onRemove: (id: string) => void;
}>) {
  const t = useT();
  const named = useEntityName(
    lookup ?? "company",
    lookup && label === undefined ? id : null,
  );
  const shown = label ?? named.name ?? id;
  return (
    <li>
      <span>{shown}</span>
      <button
        type="button"
        className="btn-link"
        aria-label={t("filters.removeRecord", { record: shown })}
        onClick={() => onRemove(id)}
      >
        <X aria-hidden="true" />
      </button>
    </li>
  );
}

/** The ids a value holds, whichever shape the operator gave it. */
export function chosenIDs(value: LeafValue): readonly string[] {
  if (Array.isArray(value)) {
    return value.map(String).filter((id) => id !== "");
  }
  return typeof value === "string" && value !== "" ? [value] : [];
}

/** The typed words are never the value, only an id picked from an answer, so
 *  no clause written here is one the engine would refuse. */
export function SearchedRecordValue({
  many,
  value,
  onChange,
  label,
  placeholder,
}: Readonly<{
  label?: string;
  placeholder: string;
  many: boolean;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
}>) {
  const t = useT();
  const searchName = label ?? t("filters.searchRecords");
  const [query, setQuery] = useState("");
  // Names the reader just picked, shown before the name lookup has answered.
  const [names, setNames] = useState<ReadonlyMap<string, string>>(new Map());
  // Only the reader's own pick moves focus; a clause restored from a saved
  // view takes none.
  const [picked, setPicked] = useState(false);
  const box = useRef<HTMLInputElement>(null);
  const change = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const { results, pending, failed } = useDebouncedSearch(
    searchCompanies,
    query,
  );
  const ids = chosenIDs(value);

  function pick(option: SearchResult) {
    setNames((was) => new Map(was).set(option.value, option.label));
    setPicked(true);
    setQuery("");
    onChange(many ? [...ids, option.value] : option.value);
  }
  function drop(id: string) {
    onChange(many ? ids.filter((held) => held !== id) : "");
  }

  const list = useSuggestList({
    anchorRef: box,
    popupRef: popup,
    suggestions: query ? results : [],
    // Not the typed words: the server matched them by more than the name. A
    // second match here would hide the companies it found that way.
    typed: "",
    taken: (id) => ids.includes(id),
    onPick: (id) => {
      const option = results.find((result) => result.value === id);
      if (option) {
        pick(option);
      }
    },
  });

  // A single comparison names one record, so once it is named the search box
  // has nothing left to do and the name stands in its place. A list keeps the
  // box open beneath what it already holds, because the next pick is the point.
  const closed = !many && ids.length > 0;
  useLayoutEffect(() => {
    if (picked) {
      (closed ? change : box).current?.focus();
    }
  }, [closed, picked]);

  return (
    <div className="filter-value">
      <ChosenRecords
        ids={ids}
        labels={names}
        onRemove={drop}
        lookup="company"
      />
      {closed ? (
        <Button
          ref={change}
          variant="link"
          onClick={() => {
            setQuery("");
            onChange("");
          }}
        >
          {t("filters.changeRecord")}
        </Button>
      ) : (
        <>
          <label>
            <span className="sr-only">{searchName}</span>
            <input
              ref={box}
              className="input"
              {...list.fieldAria}
              autoComplete="off"
              value={query}
              placeholder={placeholder}
              onChange={(event) => {
                setQuery(event.target.value);
                list.retype();
              }}
              onFocus={list.show}
              onBlur={list.close}
              onKeyDown={list.navigate}
            />
          </label>
          <SuggestPopup list={list} />
          {/* Each state says which one it is. An empty list with no line above
              it is the failure this control exists to avoid: it reads as a
              confident "this workspace has none" for a question that never got
              an answer. */}
          {!query && <p className="t-caption">{t("filters.typeToSearch")}</p>}
          {query && pending && (
            <p className="t-caption">{t("filters.searching")}</p>
          )}
          {query && failed && (
            <p className="error">{t("filters.searchFailed")}</p>
          )}
          {query && !pending && !failed && results.length === 0 && (
            <p className="t-caption">{t("filters.noRecordMatches")}</p>
          )}
        </>
      )}
    </div>
  );
}
