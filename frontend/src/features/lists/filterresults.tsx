// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The preview's rows as a table (AC-filters-and-views-5, the reading half).
//
// It renders through the catalog's ListTable rather than a grid of its own. That
// was worth checking rather than assuming: ListTable reads as a record-list
// component, but its column type is generic over the row — key, header, and a cell
// function — so a schema-derived projection fits it without pretending to be a
// record. A second table beside it would have been the easy mistake, and
// frontend/CLAUDE.md records that this tree has twice grown a duplicate surface
// exactly that way.
//
// What this file decides is which columns to show first. The preview carries
// every non-generated column of the table, and thirty columns is not a table
// anybody reads. The rest wait unticked in the display menu.

import { useCompanyMarksByID } from "../../components/companymarks";
import { type ListColumn, ListTable } from "../../design-system/listtable";
import { ABSENT } from "../../format/absent";
import {
  formatDateAbbrev,
  formatMoneyOrAbsent,
  formatNumber,
} from "../../format/format";
import { INDUSTRY_FIELD, industryWords } from "../../format/industry";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { ENTITY, type EntityKind, isEntityKind } from "../../lib/entity";
import {
  type FilterPreview,
  type FilterResource,
  fieldLabel,
  type VocabularyField,
} from "../../lib/filterdata";
import { hasFieldWord, historyFieldLabel } from "../../lib/historyfieldlabels";
import { navigate, routeHash } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
import { ReferenceCell } from "./filterresultcell";

/** A preview row: schema-derived, so its values span every type the table holds. */
type PreviewRow = Record<string, unknown>;

/**
 * How many columns a preview opens with, before the column picker is used.
 *
 * Every other column is handed to the table unticked. So this number decides
 * whether a reader sees a table or a horizontal scrollbar.
 */
const MAX_COLUMNS = 6;

/**
 * Which column names a row, in preference order.
 *
 * `id` is last rather than first: a UUID identifies a row to the database and to
 * nobody else, so a human-readable name wins wherever the record type has one.
 */
const IDENTITY_PREFERENCE = [
  "full_name",
  "display_name",
  "name",
  "title",
  "id",
] as const;

/**
 * The columns most likely to answer "is this filter right?": what identifies the
 * row, then the fields the filter actually asked about.
 *
 * Deriving them from the filter is the point. AC-5 names five product columns for
 * contacts, and inventing my own five for each of the other record types would be
 * a list nobody agreed to; the fields a human just filtered on are the ones they
 * are checking, and that rule needs no product decision to be defensible. The
 * identity column leads because a row you cannot name is not a result you can act
 * on, and the projection's own order fills whatever is left.
 */
export function previewColumnNames(
  available: readonly string[],
  named: readonly string[],
): string[] {
  const identity = IDENTITY_PREFERENCE.find((candidate) =>
    available.includes(candidate),
  );
  const chosen: string[] = identity ? [identity] : [];
  const take = (field: string) => {
    // A filter may name a field the projection does not carry — a tag leaf reads
    // a join, not a column — so membership is checked rather than assumed.
    if (available.includes(field) && !chosen.includes(field)) {
      chosen.push(field);
    }
  };
  for (const field of named) {
    if (chosen.length >= MAX_COLUMNS) {
      return chosen;
    }
    take(field);
  }
  // A filter naming nothing the projection carries would otherwise leave a table
  // of one column, so the remaining slots fill in the projection's order —
  // except for the primary key, which is worth a column only when it is the ONLY
  // thing that names the row. A UUID beside a name it already read is a column
  // spent on nothing.
  for (const field of available) {
    if (chosen.length >= MAX_COLUMNS) {
      break;
    }
    if (field !== "id") {
      take(field);
    }
  }
  return chosen;
}

export type FilterResultsProps = Readonly<{
  preview: FilterPreview | undefined;
  /** The record type of the rows, which names the page a row opens. */
  resource: FilterResource;
  fields: readonly VocabularyField[];
  /** The fields the filter names, in the order they were written. */
  named: readonly string[];
  /** The plural noun for these rows — "contacts". Translated by the caller. */
  unit: string;
  /** Headings by column name, for a record type whose labels the vocabulary lacks. */
  columnLabels?: Readonly<Record<string, string>>;
  /** Names this table for the column widths it remembers between visits. */
  widthsKey: string;
  /** What an empty answer says, worded by the caller from the filter it ran. */
  emptyNote?: string;
  pending: boolean;
  /** How many rows match on the server, when more match than were sent. */
  total?: number;
  /** The server holds rows past these, and how to ask for the next page. */
  hasMore?: boolean;
  onLoadMore?: () => void;
  /** What the rows answer, so a new filter takes the pager back to page one. */
  scopeKey?: string;
}>;

export function FilterResults({
  preview,
  resource,
  fields,
  named,
  unit,
  columnLabels,
  widthsKey,
  emptyNote,
  pending,
  total,
  hasMore = false,
  onLoadMore,
  scopeKey,
}: FilterResultsProps) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const rows: readonly PreviewRow[] = preview?.rows ?? [];
  const fieldOf = (name: string) => fields.find((field) => field.name === name);
  // A record type's preview carries the stored table, so its picker offers a
  // column only when the product has a word for it. That keeps the audit and
  // erasure columns out. An extension kind sends only the fields it declared.
  const offered = (name: string) =>
    !isEntityKind(resource) ||
    name === "id" ||
    fieldOf(name) !== undefined ||
    columnLabels?.[name] !== undefined ||
    hasFieldWord(name);
  const shown = (preview?.columns ?? []).filter(offered);
  const opening = previewColumnNames(shown, named);
  const names = [
    ...opening,
    ...shown.filter((name) => !opening.includes(name)),
  ];
  const referenceOf = (name: string) => fieldOf(name)?.references;
  // Every company the page names, read in one batch rather than one request
  // per cell.
  const companies = useCompanyMarksByID(
    names
      .filter((name) => referenceOf(name) === "company")
      .flatMap((name) =>
        rows.flatMap((row) => {
          const value = row[name];
          return typeof value === "string" ? [value] : [];
        }),
      ),
  );
  const columns: ListColumn<PreviewRow>[] = names.map((name, index) => {
    const reference = referenceOf(name);
    return {
      key: name,
      header: columnLabels?.[name] ?? headerFor(name, fields, t),
      // The identity column is fixed, because it makes a row recognisable. The
      // picker may not hide it, and a phone shows it as the card's heading.
      fixed: index === 0,
      initiallyHidden: index >= opening.length,
      cell: (row) => {
        const value = row[name];
        return reference && typeof value === "string" && value !== "" ? (
          <ReferenceCell
            reference={reference}
            id={value}
            companies={companies}
          />
        ) : (
          cellText(row, name, fieldOf(name), { t, locale, zone })
        );
      },
    };
  });

  // A row opens the record it is, through the route every record link uses.
  // An extension kind has no record page yet, so its rows do not link.
  const kind = isEntityKind(resource) ? resource : null;
  const route = (row: PreviewRow) =>
    ENTITY[kind as EntityKind].route(String(row.id));

  return (
    <ListTable<PreviewRow>
      // Which columns start ticked follows the filter, and the table reads
      // `initiallyHidden` once, so a new opening set is a new table.
      key={`${widthsKey}:${opening.join(",")}`}
      rows={rows}
      columns={columns}
      rowKey={rowKey}
      onRowClick={kind ? (row) => navigate(route(row)) : undefined}
      rowHref={kind ? (row) => routeHash(route(row)) : undefined}
      unit={unit}
      widthsKey={widthsKey}
      emptyNote={emptyNote}
      pending={pending}
      caption={t("filters.resultsCaption")}
      total={total}
      hasMore={hasMore}
      onLoadMore={onLoadMore}
      scopeKey={scopeKey}
    />
  );
}

/**
 * A row's React key.
 *
 * Every table this endpoint previews has an `id`, so the fallback is here for the
 * type rather than for a case that arrives — and it falls back to the row's own
 * contents rather than to an index, because an index key makes React reuse the
 * wrong row when the result set shifts under a recount.
 */
function rowKey(row: PreviewRow): string {
  return typeof row.id === "string" ? row.id : JSON.stringify(row);
}

/**
 * A column's header: the same word the field picker gives a filterable field,
 * so a clause and the column it selects call the field by one name, and the
 * History tab's word for any other column the projection carries.
 */
function headerFor(
  name: string,
  fields: readonly VocabularyField[],
  t: (key: MessageKey) => string,
): string {
  const known = fields.find((field) => field.name === name);
  return known ? fieldLabel(known, t) : historyFieldLabel(name, t);
}

type Reading = Readonly<{
  t: Translator;
  locale: Locale;
  zone: string;
}>;

/**
 * One cell, as text, read by the field's type the way the record lists read it.
 *
 * A null reads as a dash rather than as blank, because an empty cell cannot be
 * told apart from a column that failed to render — and "nothing here" is exactly
 * what a reader checking an `exists` clause needs to see. An object is
 * stringified rather than dropped: a jsonb column is rare in a preview, and
 * showing its shape beats showing nothing.
 */
function cellText(
  row: PreviewRow,
  name: string,
  field: VocabularyField | undefined,
  reading: Reading,
): string {
  const value = row[name];
  const { t, locale, zone } = reading;
  if (value === null || value === undefined) {
    return ABSENT;
  }
  if (typeof value === "boolean") {
    return t(value ? "field.yes" : "field.no");
  }
  if (typeof value === "number") {
    return numberText(value, row, name, field, locale);
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  const text = String(value);
  if (isDateColumn(name, field)) {
    return dateText(text, locale, zone);
  }
  return name === INDUSTRY_FIELD ? industryWords(t, text) : text;
}

// The vocabulary types only the filterable columns; the schema names the rest of
// its timestamps and days `*_at`, `*_date` or `*_until`.
function isDateColumn(name: string, field: VocabularyField | undefined) {
  return field?.type === "date" || /_(at|date|until)$/.test(name);
}

// The preview sends a `date` column as its day and a timestamp as an instant,
// which is read in the reader's zone.
const DAY_OR_INSTANT = /^\d{4}-\d{2}-\d{2}(T|$)/;

function dateText(text: string, locale: Locale, zone: string): string {
  return DAY_OR_INSTANT.test(text)
    ? formatDateAbbrev(text, locale, zone)
    : text;
}

/**
 * A figure in the reader's grouping. Money is shown only where its currency is
 * known: a custom field's own, or the `currency` a deal's `*_minor` columns pair
 * with. Any other count of minor units stays as stored, since its scale is
 * unknown without its currency.
 */
function numberText(
  value: number,
  row: PreviewRow,
  name: string,
  field: VocabularyField | undefined,
  locale: Locale,
): string {
  if (field?.type === "currency") {
    return formatMoneyOrAbsent(value, field.currency, locale);
  }
  if (name.endsWith("_minor") && typeof row.currency === "string") {
    return formatMoneyOrAbsent(value, row.currency, locale);
  }
  return field?.type === "number" ? formatNumber(value, locale) : String(value);
}
