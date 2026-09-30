// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One list, opened: what it is for, who looks after it, how many of its
// members this reader can see, the members themselves, why each is there, and
// what changed. The members are the record list's own rows, narrowed by
// list_id, so the page is never one request per member.

import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DataTable } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { throwProblem, useMe } from "./common";
import { ListHistoryPanel } from "./listhistory";
import {
  ListHealthBadge,
  ListKindBadge,
  RECORD_TYPE_LABEL,
  SHARING_LABEL,
} from "./listlibrary";
import {
  type List,
  type ListRecordType,
  useArchiveList,
  useList,
  useListsAvailable,
  useUpdateList,
} from "./lists.queries";
import { ListSettingsAction } from "./listsettings";
import { ListWhy } from "./listwhy";
import "./lists.css";

/** Where each record type's rows are read, and the screen a row opens. */
const MEMBER_SOURCES = {
  contact: { path: "/contacts", screen: "contacts" },
  company: { path: "/companies", screen: "companies" },
  deal: { path: "/deals", screen: "deals" },
  lead: { path: "/leads", screen: "leads" },
} as const;

type MemberSource = keyof typeof MEMBER_SOURCES;

function isMemberSource(type: ListRecordType): type is MemberSource {
  return type in MEMBER_SOURCES;
}

const MEMBER_PAGE = 50;

export function ListScreen({ listID }: Readonly<{ listID?: string }>) {
  const t = useT();
  const available = useListsAvailable();
  if (!available || !listID) {
    return <p className="wrap lists-note">{t("lists.unavailable")}</p>;
  }
  return <ListBody listID={listID} />;
}

function ListBody({ listID }: Readonly<{ listID: string }>) {
  const t = useT();
  const list = useList(listID);
  if (list.isPending) {
    return null;
  }
  if (list.isError) {
    return <p className="wrap lists-note">{t("lists.gone")}</p>;
  }
  return (
    <div className="wrap lists-page">
      <ListHead list={list.data} />
      <ListNotices list={list.data} />
      <MembersPanel list={list.data} />
      <ListHistoryPanel listID={list.data.id} />
    </div>
  );
}

function ListHead({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const archive = useArchiveList();
  const lastExport = list.dependencies?.[0];
  return (
    <header className="lists-head">
      <div className="lists-head-title">
        <Heading size="xlarge">{list.name}</Heading>
        <ListKindBadge list={list} />
        <ListHealthBadge list={list} />
      </div>
      {list.purpose && <p className="lists-note">{list.purpose}</p>}
      <p className="t-caption">
        {t("lists.head.facts", {
          type: t(RECORD_TYPE_LABEL[list.entity_type]),
          visible:
            list.visible_count == null
              ? "—"
              : formatNumber(list.visible_count, locale),
          sharing: t(SHARING_LABEL[list.sharing]),
          steward: list.steward_name ?? t("lists.noSteward"),
        })}
      </p>
      {lastExport && (
        <p className="t-caption">
          {plural("lists.head.exported", list.dependencies?.length ?? 0, {
            count: formatNumber(list.dependencies?.length ?? 0, locale),
            when: formatDateTime(lastExport.occurred_at, locale, viewerZone()),
          })}
        </p>
      )}
      {list.can_edit && !list.archived_at && (
        <div className="card-actions">
          <ListSettingsAction list={list} />
          <Button
            variant="ghost"
            pending={archive.isPending}
            onClick={() => archive.mutate({ id: list.id, archive: true })}
          >
            {t("lists.archive")}
          </Button>
        </div>
      )}
    </header>
  );
}

/** What the list says about itself: archived, broken, or looked after by nobody. */
function ListNotices({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const me = useMe();
  const update = useUpdateList();
  const archive = useArchiveList();
  if (list.archived_at) {
    return (
      <Callout
        tone="info"
        title={t("lists.archived.title")}
        actions={
          list.can_edit ? (
            <Button
              pending={archive.isPending}
              onClick={() => archive.mutate({ id: list.id, archive: false })}
            >
              {t("lists.restore")}
            </Button>
          ) : undefined
        }
      >
        {t("lists.archived.body")}
      </Callout>
    );
  }
  if (list.health === "invalid") {
    return (
      <Callout tone="danger" title={t("lists.invalid.title")}>
        {t("lists.invalid.body")}
      </Callout>
    );
  }
  if (list.health !== "ownerless") {
    return null;
  }
  const myID = me.data?.user.id;
  return (
    <Callout
      tone="warning"
      title={t("lists.ownerless.title")}
      actions={
        list.can_edit && myID ? (
          <Button
            pending={update.isPending}
            onClick={() =>
              update.mutate({
                id: list.id,
                version: list.version,
                stewardId: myID,
              })
            }
          >
            {t("lists.ownerless.takeOver")}
          </Button>
        ) : undefined
      }
    >
      {t("lists.ownerless.body")}
    </Callout>
  );
}

type MemberRow = Readonly<Record<string, unknown> & { id: string }>;

function MembersPanel({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const [why, setWhy] = useState<MemberRow | null>(null);
  if (!isMemberSource(list.entity_type)) {
    return (
      <Panel title={t("lists.members.title")}>
        <PanelBody>
          <p className="lists-note">{t("lists.members.projects")}</p>
        </PanelBody>
      </Panel>
    );
  }
  const source = MEMBER_SOURCES[list.entity_type];
  return (
    <Panel title={t("lists.members.title")}>
      <PanelBody>
        <MemberRows
          list={list}
          source={list.entity_type}
          onWhy={setWhy}
          onOpen={(row) => navigate({ screen: source.screen, id: row.id })}
        />
      </PanelBody>
      <ListWhy
        list={list}
        record={why ? { id: why.id, name: recordName(why, t) } : null}
        onClose={() => setWhy(null)}
      />
    </Panel>
  );
}

function MemberRows({
  list,
  source,
  onWhy,
  onOpen,
}: Readonly<{
  list: List;
  source: MemberSource;
  onWhy: (row: MemberRow) => void;
  onOpen: (row: MemberRow) => void;
}>) {
  const t = useT();
  const members = useInfiniteQuery({
    queryKey: ["lists", "members", list.id, list.version],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET(MEMBER_SOURCES[source].path, {
        params: {
          query: { list_id: list.id, limit: MEMBER_PAGE, cursor: pageParam },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
  const rows: MemberRow[] =
    members.data?.pages.flatMap((page) => page.data as MemberRow[]) ?? [];
  return (
    <SurfaceState
      state={
        members.isPending
          ? "loading"
          : members.isError
            ? "unavailable"
            : rows.length > 0
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
      <DataTable
        label={t("lists.members.title")}
        rows={rows}
        rowKey={(row) => row.id}
        onRowClick={onOpen}
        columns={[
          {
            key: "name",
            header: t("lists.col.name"),
            grow: true,
            render: (row) => recordName(row, t),
          },
          {
            key: "why",
            header: t("lists.members.whyColumn"),
            render: (row) => (
              <span className="cell-actions">
                <Button
                  variant="ghost"
                  onClick={(event) => {
                    event.stopPropagation();
                    onWhy(row);
                  }}
                >
                  {t("lists.members.why")}
                </Button>
              </span>
            ),
          },
        ]}
      />
      {members.hasNextPage && (
        <Button
          variant="ghost"
          pending={members.isFetchingNextPage}
          onClick={() => members.fetchNextPage()}
        >
          {t("lists.members.more")}
        </Button>
      )}
    </SurfaceState>
  );
}

/** A record's name, whichever field its type names itself by. */
export function recordName(
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
