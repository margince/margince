// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One list, opened: what it is for, who looks after it, how many of its
// members this reader can see, the members themselves with what put each
// there, and what changed. The members are the record list's own rows, narrowed by
// list_id, so the page is never one request per member (listmembers.tsx).

import { type ReactNode, useEffect, useMemo, useRef } from "react";
import { navigate } from "../app/router";
import { Button, PendingBody } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { useMe } from "./common";
import { customColumnLabel, useFilterVocabulary } from "./filterdata";
import { EditFilterAction, mayEditFilter } from "./filterlistedit";
import { filterSentence, useSentenceWords } from "./filtersentence";
import { ListChangeSummary } from "./listchanges";
import { ListHistoryPanel } from "./listhistory";
import {
  ListHealthBadge,
  ListKindBadge,
  RECORD_TYPE_LABEL,
} from "./listlibrary";
import { useMemberColumns } from "./listmembercolumns";
import { MEMBER_SOURCES, MemberRows, type MemberSource } from "./listmembers";
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
import "./lists.css";
import { decode, rootGroup } from "./segmentpredicate";

function isMemberSource(type: ListRecordType): type is MemberSource {
  return type in MEMBER_SOURCES;
}

export function ListScreen({ listID }: Readonly<{ listID: string }>) {
  const t = useT();
  const available = useListsAvailable();
  if (!available) {
    return (
      <ListState>
        <p className="lists-note">{t("lists.unavailable")}</p>
      </ListState>
    );
  }
  return <ListBody listID={listID} />;
}

/**
 * The page before it has a list to name. It heads itself, so it still prints
 * the one heading a page owes a reader navigating by heading.
 */
function ListState({ children }: Readonly<{ children: ReactNode }>) {
  const t = useT();
  return (
    <div className="wrap lists-page">
      <Heading size="xlarge">{t("lists.page")}</Heading>
      {children}
    </div>
  );
}

function ListBody({ listID }: Readonly<{ listID: string }>) {
  const t = useT();
  const list = useList(listID);
  useVisitOnce(listID, list.isSuccess && list.isFetchedAfterMount);
  if (list.isPending) {
    return (
      <ListState>
        <PendingBody label={t("lists.loading")} lines={6} />
      </ListState>
    );
  }
  if (list.isError) {
    return (
      <ListState>
        <p className="lists-note">{t("lists.gone")}</p>
      </ListState>
    );
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
      <ListFilterLine list={list} />
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
 * What a Live List selects, as one sentence, once the vocabulary names its
 * fields: a bare count cannot finish "where …". A Shortlist has no filter, and
 * a definition this page cannot read says nothing rather than something wrong.
 */
function ListFilterLine({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const words = useSentenceWords();
  const live = list.list_type === "dynamic";
  const vocabulary = useFilterVocabulary(list.entity_type, live);
  const tree = live ? decode(list.definition) : null;
  const fields = vocabulary.data?.fields;
  if (tree === null || fields === undefined) {
    return null;
  }
  return (
    <p className="t-caption">
      {t("lists.filterLine", {
        records: t(RECORD_TYPE_LABEL[list.entity_type]),
        sentence: filterSentence(rootGroup(tree), fields, words),
      })}
    </p>
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
  const joined = useMemo(
    () => new Set(list.joined_since_visit ?? []),
    [list.joined_since_visit],
  );
  const columns = useMemberColumns(list, joined);
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
          columns={columns}
        />
      </PanelBody>
    </Panel>
  );
}
