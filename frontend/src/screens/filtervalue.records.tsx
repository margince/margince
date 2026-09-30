// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The records an id clause already names, and the search that finds a company
// to add: the half of the filter value control that speaks in records rather
// than in scalars.

import { X } from "lucide-react";
import { useState } from "react";
import {
  type SearchResult,
  useDebouncedSearch,
} from "../design-system/debouncedsearch";
import { useT } from "../i18n";
import "./filterbuilder.css";
import { searchCompanies } from "./filterreference";
import type { LeafValue } from "./segmentpredicate";

/**
 * The ids a list clause already names, and a way to drop one.
 *
 * Shown as the labels they were picked under where this session knows them. A
 * clause restored from a saved view carries ids and no names, so the id shows —
 * a reader can still see which records the clause holds and remove one, where
 * hiding them would leave a list they cannot inspect.
 */
export function ChosenRecords({
  ids,
  labels,
  onRemove,
}: Readonly<{
  ids: readonly string[];
  labels: ReadonlyMap<string, string>;
  onRemove: (id: string) => void;
}>) {
  const t = useT();
  if (ids.length === 0) {
    return null;
  }
  return (
    <ul className="filter-chosen">
      {ids.map((id) => (
        <li key={id}>
          <span>{labels.get(id) ?? id}</span>
          <button
            type="button"
            className="btn-link"
            aria-label={t("filters.removeRecord", {
              record: labels.get(id) ?? id,
            })}
            onClick={() => onRemove(id)}
          >
            <X aria-hidden="true" />
          </button>
        </li>
      ))}
    </ul>
  );
}

/** The ids a value holds, whichever shape the operator gave it. */
export function chosenIDs(value: LeafValue): readonly string[] {
  if (Array.isArray(value)) {
    return value.map(String).filter((id) => id !== "");
  }
  return typeof value === "string" && value !== "" ? [value] : [];
}

/**
 * The one reference too large to list: a company, found by typing part of
 * its name.
 *
 * A box was the previous answer and it was the wrong one for the reason this
 * whole surface exists: a uuid typed wrong compiles, matches nothing, and reads
 * as "no companies match" rather than as a mistake. Nobody types a uuid
 * correctly from memory anyway, so the box was asking for a paste from another
 * screen.
 *
 * The typed words are NEVER the value. The input searches and the value is only
 * ever an id the reader picked out of an answer, which is what makes it
 * impossible to compose a clause the engine would refuse.
 *
 * Once chosen, the name stands in place of the search box with a way back to it:
 * a picker that kept showing its search box would leave the reader unsure
 * whether their choice took.
 */
export function SearchedRecordValue({
  many,
  value,
  onChange,
  label,
}: Readonly<{
  label?: string;
  many: boolean;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
}>) {
  const t = useT();
  const searchName = label ?? t("filters.searchRecords");
  const [query, setQuery] = useState("");
  // Names for the ids chosen in THIS session. A clause restored from a saved
  // view carries ids and nothing else, so this stays empty for them and the id
  // shows instead — see ChosenRecords.
  const [names, setNames] = useState<ReadonlyMap<string, string>>(new Map());
  const { results, pending, failed } = useDebouncedSearch(
    searchCompanies,
    query,
  );
  const ids = chosenIDs(value);

  function remember(option: SearchResult) {
    setNames((was) => new Map(was).set(option.value, option.label));
  }
  function pick(option: SearchResult) {
    remember(option);
    setQuery("");
    onChange(many ? [...ids, option.value] : option.value);
  }
  function drop(id: string) {
    onChange(many ? ids.filter((held) => held !== id) : "");
  }

  // A single comparison names one record, so once it is named the search box
  // has nothing left to do and the name stands in its place. A list keeps the
  // box open beneath what it already holds, because the next pick is the point.
  const closed = !many && ids.length > 0;

  return (
    <div className="filter-value">
      <ChosenRecords ids={ids} labels={names} onRemove={drop} />
      {closed ? (
        <button
          type="button"
          className="btn-link"
          onClick={() => {
            setQuery("");
            onChange("");
          }}
        >
          {t("filters.changeRecord")}
        </button>
      ) : (
        <>
          <label>
            <span className="sr-only">{searchName}</span>
            <input
              className="input"
              value={query}
              placeholder={t("filters.searchRecords")}
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          {/* Each state says which one it is. An empty list with no line above
              it is the failure this control exists to avoid: it reads as a
              confident "this workspace has none" for a question that never got
              an answer. */}
          {!query && <p>{t("filters.typeToSearch")}</p>}
          {query && pending && <p>{t("filters.searching")}</p>}
          {query && failed && (
            <p className="error">{t("filters.searchFailed")}</p>
          )}
          {query && !pending && !failed && results.length === 0 && (
            <p>{t("filters.noRecordMatches")}</p>
          )}
          {results.map((option) => (
            <button
              type="button"
              key={option.value}
              className="btn-link"
              onClick={() => pick(option)}
            >
              {option.label}
            </button>
          ))}
        </>
      )}
    </div>
  );
}
