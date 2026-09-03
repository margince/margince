import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import type { ListColumn } from "../design-system/listtable";
import { RowTags } from "../design-system/rowtags";
import { formatDateAbbrev } from "../format/format";
import type { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { OwnerName } from "./entityref";
import type { ListState, ViewSpec } from "./listquery";

type RowTag = components["schemas"]["RowTag"];

/**
 * The columns and views every owner-scoped record list shares (people,
 * companies, leads), defined ONCE.
 *
 * Three lists carried three copies of the Owner column, and when the column
 * was fixed on People and Companies (it had rendered "typed by a person" for
 * every human-captured row) the Leads copy kept the bug. A column that exists
 * on two lists is defined here; a screen adds only what is specific to its
 * record.
 */

/** A row that can be owned and was created at some point. */
export type OwnedRecord = {
  owner_id?: string | null;
  created_at?: string;
  last_activity_at?: string | null;
};

/** A row that carries tags. The three record lists and the pipeline all do. */
export type TaggedRecord = {
  tags?: readonly RowTag[];
};

type Translate = ReturnType<typeof useT>;
type Locale = ReturnType<typeof useLocale>["locale"];

/**
 * The Tags column: how this record is filed.
 *
 * Not sortable. Sorting a multi-value cell has to pick one of its values to
 * sort by, and whichever it picks the order is a claim about the row that the
 * row does not make — filtering by a tag is the question a reader actually
 * has, and the filter answers it exactly.
 */
export function tagsColumn<Row extends TaggedRecord>(
  t: Translate,
): ListColumn<Row> {
  return {
    key: "tags",
    header: t("tags.columnHeader"),
    cell: (row) => <RowTags tags={row.tags} />,
  };
}

/** The Owner column: whose record this is, sortable by owner_id. */
export function ownerColumn<Row extends OwnedRecord>(
  t: Translate,
): ListColumn<Row> {
  return {
    key: "owner",
    header: t("list.owner"),
    cell: (row) => (
      <OwnerName ownerId={row.owner_id} unowned={t("list.unowned")} />
    ),
    sort: "owner_id",
  };
}

/** The Created column, in the record zone, sortable by created_at. */
export function createdColumn<Row extends OwnedRecord>(
  t: Translate,
  locale: Locale,
  recordZone: string,
): ListColumn<Row> {
  return {
    key: "created",
    header: t("list.created"),
    cell: (row) => (
      <span className="t-caption">
        {row.created_at
          ? formatDateAbbrev(row.created_at, locale, recordZone)
          : ""}
      </span>
    ),
    sort: "created_at",
  };
}

/**
 * The Last activity column: the timeline's clock, maintained in the schema so
 * the server can sort on it; empty until something has happened.
 */
export function lastActivityColumn<Row extends OwnedRecord>(
  t: Translate,
  locale: Locale,
  recordZone: string,
): ListColumn<Row> {
  return {
    key: "lastActivity",
    header: t("list.lastActivity"),
    cell: (row) => (
      <span className="t-caption">
        {row.last_activity_at
          ? formatDateAbbrev(row.last_activity_at, locale, recordZone)
          : ""}
      </span>
    ),
    sort: "last_activity_at",
  };
}

/**
 * The views every record list opens with: everything newest-first, and —
 * for a signed-in reader — theirs. A screen appends its own (A–Z on its
 * name column, lifecycle cuts, score cuts).
 */
export function standardViews(
  viewerId: string | undefined,
  options: Readonly<{ sort?: string; mineFirst?: boolean }> = {},
): readonly ViewSpec[] {
  const sort = options.sort ?? "-created_at";
  const all: ViewSpec = { label: "list.viewAll", sort };
  if (!viewerId) return [all];

  const mine: ViewSpec = {
    label: "list.viewMine",
    sort,
    filters: { owner_id: viewerId },
  };
  return options.mineFirst ? [mine, all] : [all, mine];
}

/**
 * What an empty "Mine" tab says, and the way back to everything.
 *
 * The other half of `standardViews`, and it belongs beside it: a screen that
 * renders the Mine tab owes the reader this sentence, because a bare "no
 * matches" over a filter they may not remember turning reads as an empty
 * workspace. It was spelled on the leads queue alone, so the companies and
 * contacts lists — same tab, same emptiness a reader reaches in one click —
 * said nothing at all.
 *
 * Undefined unless Mine is what is actually on screen. Under any other
 * narrowing the table's own line is the right one, and a sentence about
 * ownership would name a cause that is not the cause.
 *
 * "Show all" drops the owner filter and nothing else, which is what makes it
 * worth having beside the table's own "clear filters": one undoes the tab, the
 * other undoes every dial the reader has turned.
 */
export function mineEmptyNote<Row>({
  t,
  state,
  viewerId,
  unit,
}: Readonly<{
  t: Translate;
  state: ListState<Row>;
  viewerId: string | undefined;
  /** The plural noun for these rows, so the sentence names them. */
  unit: MessageKey;
}>): ReactNode | undefined {
  if (!viewerId || state.query.filters.owner_id !== viewerId) {
    return undefined;
  }
  return (
    <span>
      {t("list.emptyMine", { unit: t(unit) })}{" "}
      <Button
        small
        onClick={() =>
          state.setQuery((query) => {
            const { owner_id: _mine, ...rest } = query.filters;
            return { ...query, filters: rest };
          })
        }
      >
        {t("list.showAll")}
      </Button>
    </span>
  );
}
