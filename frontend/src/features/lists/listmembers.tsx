// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A list's members, and what a reader can do to them. Every verb acts on the
// members the reader selected — explicit records, never "whoever matches
// later" — through the bulk dialog every record list uses.

import {
  skipToken,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api, FIRST_PAGE } from "../../api/client";
import { BULK_MAX_ITEMS } from "../../components/bulkchangeapi";
import { BulkVerbs } from "../../components/bulkverbs";
import { useRowSelection } from "../../design-system/listselection";
import {
  type ListColumn,
  type ListSelection,
  ListTable,
} from "../../design-system/listtable";
import { Panel } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { downloadBytes, filenameFromDisposition } from "../../lib/download";
import {
  LISTS_KEY,
  type List,
  type ListMember,
  type ListRecordType,
  listMembersAmong,
} from "../../lib/lists.queries";
import { throwProblem, unwrap } from "../../lib/problem";
import "./lists.css";
import "./listsection.css";

/** Where each record type's rows are read. `ENTITY` names the page a row opens. */
export const MEMBER_SOURCES = {
  contact: { path: "/contacts", unit: "unit.contacts" },
  company: { path: "/companies", unit: "unit.companies" },
  deal: { path: "/deals", unit: "unit.deals" },
  lead: { path: "/leads", unit: "unit.leads" },
} as const satisfies Record<string, { path: string; unit: MessageKey }>;

type MemberSource = keyof typeof MEMBER_SOURCES;

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
  const data = unwrap(
    await api.GET(MEMBER_SOURCES[source].path, {
      params: { query: { list_id: listId, limit, cursor } },
    }),
  );
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

export function isMemberSource(type: ListRecordType): type is MemberSource {
  return type in MEMBER_SOURCES;
}

/**
 * The members this reader can see, a page at a time. The page's head and its
 * Members panel both read them, and React Query serves both from one request.
 * A list of a type the record lists do not serve reads nothing.
 */
function useListMembers(list: List, source: MemberSource | undefined) {
  const members = useInfiniteQuery({
    queryKey: ["lists", "members", list.id, list.version],
    initialPageParam: FIRST_PAGE,
    queryFn:
      source === undefined
        ? skipToken
        : ({ pageParam }) =>
            listedPage(source, list.id, pageParam ?? undefined),
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
  const shown: MemberRow[] =
    members.data?.pages.flatMap((page) => [...page.data]) ?? [];
  const state: SectionState = members.isPending
    ? "loading"
    : members.isError
      ? "unavailable"
      : shown.length > 0
        ? "ready"
        : "empty";
  return { members, shown, state };
}

/**
 * The list's members as a CSV file, through the one export the filters use.
 * It is offered once the members are read and there are some to export. It is
 * `awaited` while a list whose members can be read is still reading them.
 */
export function useListExport(list: List) {
  const client = useQueryClient();
  const source = isMemberSource(list.entity_type)
    ? list.entity_type
    : undefined;
  const { state } = useListMembers(list, source);
  const run = useMutation({
    mutationFn: async (
      input: Readonly<{ listId: string; recordType: ListRecordType }>,
    ) => {
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
          `${input.recordType}-export.csv`,
        ),
        "text/csv",
      );
    },
    // The server logs each export on the list, which its Exported fact reads.
    onSuccess: (_, input) =>
      client.invalidateQueries({ queryKey: [LISTS_KEY, "one", input.listId] }),
  });
  return {
    offered: state === "ready",
    awaited: source !== undefined && state === "loading",
    run,
  };
}

/**
 * The list's members as one Panel. The table is the Panel's own content, so
 * the surface sheds its box (listsection.css) and the Panel is the one card.
 */
export function MembersPanel({
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
  const { members, shown, state } = useListMembers(list, source);
  const chosen = useMemberSelection(list, source, shown);
  return (
    <Panel className="listsection" title={t("lists.members.title")}>
      <SurfaceState
        state={state}
        emptyLabel={
          list.list_type === "dynamic"
            ? t("lists.members.emptyLive")
            : t("lists.members.emptyShortlist")
        }
        loadingLabel={t("lists.members.loading")}
        loadingLines={5}
      >
        <ListTable<MemberRow>
          rows={shown}
          columns={columns}
          rowKey={(row) => row.id}
          onRowClick={onOpen}
          unit={t(MEMBER_SOURCES[source].unit)}
          selection={chosen}
          hasMore={members.hasNextPage}
          onLoadMore={() => members.fetchNextPage()}
          pending={members.isFetchingNextPage}
        />
      </SurfaceState>
    </Panel>
  );
}

// A row is known by the version it was read at, which is what the bulk change
// holds each record to.
function useMemberSelection(
  list: List,
  source: MemberSource,
  shown: readonly MemberRow[],
): ListSelection<MemberRow> {
  const t = useT();
  const chosen = useRowSelection({
    rows: shown,
    keyOf: (row) => row.id,
    limit: BULK_MAX_ITEMS,
  });
  const shortlist =
    list.list_type === "static" && list.can_edit && !list.archived_at
      ? { id: list.id, name: list.name }
      : undefined;
  return {
    selected: chosen.selected,
    limit: chosen.limit,
    selectable: (row) => row.archived_at == null,
    onChange: chosen.onChange,
    label: (row) => t("bulk.selectRow", { name: memberName(row, t) }),
    bar: (
      <BulkVerbs
        recordType={source}
        rows={chosen.selectedRows.map((row) => ({
          id: row.id,
          version: row.version,
          label: memberName(row, t),
        }))}
        shortlist={shortlist}
        onDone={chosen.clear}
      />
    ),
  };
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
