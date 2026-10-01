// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One list, opened: what it is for, who looks after it, how many of its
// members this reader can see, the members themselves, why each is there, and
// what changed. The members are the record list's own rows, narrowed by
// list_id, so the page is never one request per member (listmembers.tsx).

import { useEffect, useMemo, useRef, useState } from "react";
import { navigate } from "../app/router";
import { Badge, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { useMe } from "./common";
import { customColumnLabel } from "./filterdata";
import { EditFilterAction, mayEditFilter } from "./filterlistedit";
import { ListChangeSummary } from "./listchanges";
import { ListHistoryPanel } from "./listhistory";
import {
  ListHealthBadge,
  ListKindBadge,
  RECORD_TYPE_LABEL,
} from "./listlibrary";
import {
  MEMBER_SOURCES,
  type MemberRow,
  MemberRows,
  type MemberSource,
  memberName,
} from "./listmembers";
import { ArchiveListAction } from "./listrules";
import {
  type List,
  type ListRecordType,
  useArchiveList,
  useList,
  useListsAvailable,
  useUpdateList,
  useVisitList,
} from "./lists.queries";
import { ListSettingsAction } from "./listsettings";
import { useListAudienceLabel } from "./listsharing";
import { ListWhy } from "./listwhy";
import "./lists.css";

function isMemberSource(type: ListRecordType): type is MemberSource {
  return type in MEMBER_SOURCES;
}

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
  useVisitOnce(listID, list.isSuccess && list.isFetchedAfterMount);
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
      <ListHistoryPanel
        listID={list.data.id}
        live={list.data.list_type === "dynamic"}
      />
    </div>
  );
}

/**
 * Records one visit per opened list, once this page has read the list itself
 * rather than a cached copy. The server counts "since your last visit" from
 * the visit before the one in progress, so the read and the visit may land in
 * either order and the page shows the same counts.
 */
function useVisitOnce(listID: string, readThisMount: boolean) {
  const { mutate } = useVisitList();
  const visited = useRef<string | null>(null);
  useEffect(() => {
    if (readThisMount && visited.current !== listID) {
      visited.current = listID;
      mutate(listID);
    }
  }, [listID, readThisMount, mutate]);
}

function ListHead({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const audienceOf = useListAudienceLabel();
  const exports = (list.dependencies ?? []).filter(
    (use) => use.kind === "export",
  );
  const lastExport = exports[0];
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
          sharing: audienceOf(list),
          steward: list.steward_name ?? t("lists.noSteward"),
        })}
      </p>
      <ListCheckLine list={list} />
      {lastExport && (
        <p className="t-caption">
          {plural("lists.head.exported", exports.length, {
            count: formatNumber(exports.length, locale),
            when: formatDateTime(lastExport.occurred_at, locale, viewerZone()),
          })}
        </p>
      )}
      {list.can_edit && !list.archived_at && (
        <div className="card-actions">
          <ListSettingsAction list={list} />
          <EditFilterAction list={list} />
          <ArchiveListAction list={list} />
        </div>
      )}
    </header>
  );
}

/**
 * When a Live List was last checked for who joined and left, and what it
 * gained and lost since the reader's last visit. A Shortlist says nothing.
 */
function ListCheckLine({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const { locale } = useLocale();
  if (list.list_type !== "dynamic") {
    return null;
  }
  const check = list.last_check;
  const pulse = list.since_last_visit;
  const changes = list.changes_since_visit;
  const type = list.entity_type;
  const when = check
    ? formatDateTime(check.checked_at, locale, viewerZone())
    : "";
  return (
    <>
      <p className="t-caption">
        {!check
          ? t("lists.head.notChecked")
          : check.outcome === "too_large"
            ? t("lists.head.tooLarge", { when })
            : t("lists.head.lastChecked", { when })}
      </p>
      {changes ? (
        <ListChangeSummary
          summary={changes}
          onOpen={
            isMemberSource(type)
              ? (id) => navigate({ screen: MEMBER_SOURCES[type].screen, id })
              : undefined
          }
        />
      ) : (
        pulse &&
        pulse.entered + pulse.left > 0 && (
          <p className="t-caption">
            {t("lists.head.pulse", {
              entered: formatNumber(pulse.entered, locale),
              left: formatNumber(pulse.left, locale),
            })}
          </p>
        )
      )}
    </>
  );
}

/**
 * What the list says about itself: archived, broken, looked after by nobody,
 * or filtering on a field that was retired.
 */
function ListNotices({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const plural = usePlural();
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
      <Callout
        tone="danger"
        title={t("lists.invalid.title")}
        actions={
          mayEditFilter(list) ? <EditFilterAction list={list} /> : undefined
        }
      >
        {t("lists.invalid.body")}
      </Callout>
    );
  }
  if (list.health === "retired_field") {
    const fields = list.retired_fields ?? [];
    return (
      <Callout
        tone="warning"
        title={t("lists.retiredField.title")}
        actions={
          mayEditFilter(list) ? <EditFilterAction list={list} /> : undefined
        }
      >
        {plural("lists.retiredField.body", fields.length, {
          fields: fields.map(customColumnLabel).join(", "),
        })}
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

function MembersPanel({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const [why, setWhy] = useState<MemberRow | null>(null);
  const joined = useMemo(
    () => new Set(list.joined_since_visit ?? []),
    [list.joined_since_visit],
  );
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
          onOpen={(row) => navigate({ screen: source.screen, id: row.id })}
          columns={[
            {
              key: "name",
              header: t("lists.col.name"),
              fixed: true,
              cell: (row) =>
                joined.has(row.id) ? (
                  <span className="lists-member-name">
                    {memberName(row, t)}
                    <Badge tone="accent">{t("lists.members.new")}</Badge>
                  </span>
                ) : (
                  memberName(row, t)
                ),
            },
            {
              key: "why",
              header: t("lists.members.whyColumn"),
              verbs: true,
              cell: (row) => (
                <span className="cell-actions">
                  <Button
                    variant="ghost"
                    onClick={(event) => {
                      event.stopPropagation();
                      setWhy(row);
                    }}
                  >
                    {t("lists.members.why")}
                  </Button>
                </span>
              ),
            },
          ]}
        />
      </PanelBody>
      <ListWhy
        list={list}
        record={why ? { id: why.id, name: memberName(why, t) } : null}
        onClose={() => setWhy(null)}
      />
    </Panel>
  );
}
