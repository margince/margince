// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A list's members, and what a reader can do to them. Every verb acts on the
// members the reader selected — explicit records, never "whoever matches
// later" — through the bulk dialog every record list uses.

import { useInfiniteQuery, useMutation } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import { type ListColumn, ListTable } from "../design-system/listtable";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BulkVerbs } from "./bulkverbs";
import { throwProblem } from "./common";
import { downloadBytes, filenameFromDisposition } from "./download";
import { type List, type ListMember, listMembersAmong } from "./lists.queries";

/** Where each record type's rows are read, and the screen a row opens. */
export const MEMBER_SOURCES = {
  contact: { path: "/contacts", screen: "contacts", unit: "unit.contacts" },
  company: { path: "/companies", screen: "companies", unit: "unit.companies" },
  deal: { path: "/deals", screen: "deals", unit: "unit.deals" },
  lead: { path: "/leads", screen: "leads", unit: "unit.leads" },
} as const satisfies Record<
  string,
  { path: string; screen: string; unit: MessageKey }
>;

export type MemberSource = keyof typeof MEMBER_SOURCES;

export type MemberRow = Readonly<
  Record<string, unknown> & {
    id: string;
    version?: number;
    archived_at?: string | null;
    /** What the list says about this record: its filter values, or who chose it. */
    listing?: ListMember;
  }
>;

const MEMBER_PAGE = 50;

/** The contract's bound on one bulk selection (`BulkChangePreviewRequest.items`). */
export const BULK_MAX_ITEMS = 500;

// The largest page the record lists serve, for walking the members to select.
const SELECT_ALL_PAGE = 200;

type MemberPage = Readonly<{
  data: readonly MemberRow[];
  page: Readonly<{ has_more: boolean; next_cursor?: string | null }>;
}>;

async function memberPage(
  source: MemberSource,
  listId: string,
  limit: number,
  cursor: string | undefined,
): Promise<MemberPage> {
  const { data, error } = await api.GET(MEMBER_SOURCES[source].path, {
    params: { query: { list_id: listId, limit, cursor } },
  });
  if (error) {
    throwProblem(error);
  }
  return data as MemberPage;
}

/**
 * One page of members, each with what the list says about it. Two reads, one
 * per page rather than one per member: the record rows, then the list's own
 * answer for exactly those records.
 */
async function listedPage(
  source: MemberSource,
  listId: string,
  cursor: string | undefined,
): Promise<MemberPage> {
  const page = await memberPage(source, listId, MEMBER_PAGE, cursor);
  const listed = await listMembersAmong(
    listId,
    page.data.map((row) => row.id),
  );
  return {
    ...page,
    data: page.data.map((row) => ({ ...row, listing: listed.get(row.id) })),
  };
}

/**
 * Every member the reader can see, up to the bulk cap, and whether more remain
 * past it. These are the explicit records a verb will act on.
 */
async function selectableMembers(
  source: MemberSource,
  listId: string,
): Promise<Readonly<{ rows: MemberRow[]; more: boolean }>> {
  const rows: MemberRow[] = [];
  let cursor: string | undefined;
  for (;;) {
    const page = await memberPage(source, listId, SELECT_ALL_PAGE, cursor);
    rows.push(...page.data.filter((row) => row.archived_at == null));
    cursor = page.page.next_cursor ?? undefined;
    if (!page.page.has_more || !cursor || rows.length > BULK_MAX_ITEMS) {
      return {
        rows: rows.slice(0, BULK_MAX_ITEMS),
        more: rows.length > BULK_MAX_ITEMS || page.page.has_more,
      };
    }
  }
}

export function MemberRows({
  list,
  source,
  columns,
  onOpen,
}: Readonly<{
  list: List;
  source: MemberSource;
  columns: readonly ListColumn<MemberRow>[];
  onOpen: (row: MemberRow) => void;
}>) {
  const t = useT();
  const members = useInfiniteQuery({
    queryKey: ["lists", "members", list.id, list.version],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => listedPage(source, list.id, pageParam),
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
  const shown: MemberRow[] =
    members.data?.pages.flatMap((page) => [...page.data]) ?? [];
  const chosen = useMemberSelection(list, source, shown);
  return (
    <SurfaceState
      state={
        members.isPending
          ? "loading"
          : members.isError
            ? "unavailable"
            : shown.length > 0
              ? "ready"
              : "empty"
      }
      emptyLabel={
        list.list_type === "dynamic"
          ? t("lists.members.emptyLive")
          : t("lists.members.emptyShortlist")
      }
      loadingLabel={t("lists.members.loading")}
      loadingLines={5}
    >
      <MemberTools list={list} source={source} chosen={chosen} />
      <ListTable<MemberRow>
        rows={shown}
        columns={columns}
        rowKey={(row) => row.id}
        onRowClick={onOpen}
        unit={t(MEMBER_SOURCES[source].unit)}
        selection={chosen.selection}
        hasMore={members.hasNextPage}
        onLoadMore={() => members.fetchNextPage()}
        pending={members.isFetchingNextPage}
      />
    </SurfaceState>
  );
}

type MemberSelection = ReturnType<typeof useMemberSelection>;

// The selection over the members shown and those "Select all" fetched. A row
// is known by the version it was read at, which is what the bulk change holds
// each record to.
function useMemberSelection(
  list: List,
  source: MemberSource,
  shown: readonly MemberRow[],
) {
  const t = useT();
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [fetched, setFetched] = useState<readonly MemberRow[]>([]);
  const [capped, setCapped] = useState(false);
  const known = new Map<string, MemberRow>();
  for (const row of [...fetched, ...shown]) {
    known.set(row.id, row);
  }
  const rows = [...selected].flatMap((id) => {
    const row = known.get(id);
    return row ? [row] : [];
  });
  const clear = () => {
    setSelected(new Set());
    setFetched([]);
    setCapped(false);
  };
  const selectAll = useMutation({
    mutationFn: (input: Readonly<{ listId: string }>) =>
      selectableMembers(source, input.listId),
    onSuccess: (all) => {
      setFetched(all.rows);
      setSelected(new Set(all.rows.map((row) => row.id)));
      setCapped(all.more);
    },
  });
  const shortlist =
    list.list_type === "static" && list.can_edit && !list.archived_at
      ? { id: list.id, name: list.name }
      : undefined;
  return {
    selectAll,
    capped,
    full: selected.size >= BULK_MAX_ITEMS,
    selection: {
      selected,
      selectable: (row: MemberRow) => row.archived_at == null,
      onToggle: (row: MemberRow) =>
        setSelected((prev) => {
          const next = new Set(prev);
          if (next.has(row.id)) {
            next.delete(row.id);
          } else if (next.size < BULK_MAX_ITEMS) {
            // At the cap a tick adds nothing: one change takes no more
            // records, and the notice says so. Unticking stays open.
            next.add(row.id);
          }
          return next;
        }),
      label: (row: MemberRow) =>
        t("bulk.selectRow", { name: memberName(row, t) }),
      bar: (
        <BulkVerbs
          recordType={source}
          rows={rows.map((row) => ({
            id: row.id,
            version: row.version,
            label: memberName(row, t),
          }))}
          shortlist={shortlist}
          onDone={clear}
        />
      ),
    },
  };
}

/** "Select all N members", the cap's notices, and the list's export. */
function MemberTools({
  list,
  source,
  chosen,
}: Readonly<{ list: List; source: MemberSource; chosen: MemberSelection }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const total = list.visible_count ?? 0;
  const cap = { count: formatNumber(BULK_MAX_ITEMS, locale) };
  const notices: ReactNode[] = [];
  if (chosen.capped) {
    notices.push(
      <Callout
        key="capped"
        tone="info"
        title={t("lists.members.selectAllCappedTitle")}
      >
        {plural("lists.members.selectAllCapped", BULK_MAX_ITEMS, cap)}
      </Callout>,
    );
  }
  if (chosen.full) {
    notices.push(
      <Callout
        key="full"
        tone="info"
        title={t("lists.members.selectionFullTitle")}
      >
        {plural("lists.members.selectionFull", BULK_MAX_ITEMS, cap)}
      </Callout>,
    );
  }
  return (
    <>
      <div className="card-actions">
        {total > 0 && (
          <Button
            variant="ghost"
            pending={chosen.selectAll.isPending}
            onClick={() => chosen.selectAll.mutate({ listId: list.id })}
          >
            {plural("lists.members.selectAll", total, {
              count: formatNumber(total, locale),
            })}
          </Button>
        )}
        <ExportListAction list={list} source={source} />
      </div>
      <ErrorLine inline error={chosen.selectAll.error} />
      {notices}
    </>
  );
}

/** The list's members as a CSV file, through the one export the filters use. */
function ExportListAction({
  list,
  source,
}: Readonly<{ list: List; source: MemberSource }>) {
  const t = useT();
  const run = useMutation({
    mutationFn: async (input: Readonly<{ listId: string }>) => {
      const { data, error, response } = await api.POST("/exports", {
        body: { list_id: input.listId, format: "csv" },
        parseAs: "text",
      });
      if (error) {
        throwProblem(error);
      }
      downloadBytes(
        data,
        filenameFromDisposition(
          response.headers.get("Content-Disposition"),
          `${source}-export.csv`,
        ),
        "text/csv",
      );
    },
  });
  return (
    <>
      <Button
        variant="ghost"
        pending={run.isPending}
        onClick={() => run.mutate({ listId: list.id })}
      >
        {t("filters.exportCsv")}
      </Button>
      <ErrorLine inline error={run.error} />
    </>
  );
}

/** A record's name, whichever field its type names itself by. */
export function memberName(
  row: Readonly<Record<string, unknown>>,
  t: ReturnType<typeof useT>,
): string {
  for (const field of ["full_name", "display_name", "name", "email"]) {
    const value = row[field];
    if (typeof value === "string" && value.trim() !== "") {
      return value;
    }
  }
  return t("lists.unnamed");
}
