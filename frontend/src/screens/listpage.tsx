// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One list, opened: what it is for, who looks after it, how many of its
// members this reader can see, the members themselves with what put each
// there, and what changed. The members are the record list's own rows, narrowed by
// list_id, so the page is never one request per member (listmembers.tsx).

import { useEffect, useMemo, useRef } from "react";
import { navigate } from "../app/router";
import { useArrivalFocus } from "../design-system/arrivalfocus";
import { Button, OverflowMenu } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { Fact, RecordFacts } from "../design-system/recordfacts";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { useMe } from "./common";
import { customColumnLabel, useFilterVocabulary } from "./filterdata";
import { FocusedPending, FocusedState } from "./filterhead";
import { EditFilterAction, mayEditFilter } from "./filterlistedit";
import { filterSentence, useSentenceWords } from "./filtersentence";
import { ListChangeSummary } from "./listchanges";
import { ListHistoryPanel } from "./listhistory";
import {
  ListHealthBadge,
  ListKindBadge,
  ListRecordsCount,
} from "./listlibrary";
import { useMemberColumns } from "./listmembercolumns";
import {
  isMemberSource,
  MEMBER_SOURCES,
  MembersPanel,
  useListExport,
} from "./listmembers";
import { ArchiveListAction } from "./listrules";
import {
  type List,
  useArchiveList,
  useList,
  useListsAvailable,
  useUpdateList,
  useVisitList,
} from "./lists.queries";
import { ListSettingsAction } from "./listsettings";
import { useListAudienceLabel } from "./listsharing";
import "./lists.css";
import { decode } from "./segmentpredicate";

export function ListScreen({ listID }: Readonly<{ listID: string }>) {
  const t = useT();
  const available = useListsAvailable();
  if (!available) {
    return (
      <FocusedState title={t("lists.page")} sentence={t("lists.unavailable")} />
    );
  }
  return <ListBody listID={listID} />;
}

/** The page while its list, or the session that decides lists, is read. */
export function ListPending() {
  const t = useT();
  return <FocusedPending title={t("lists.page")} label={t("lists.loading")} />;
}

function ListBody({ listID }: Readonly<{ listID: string }>) {
  const t = useT();
  const list = useList(listID);
  useVisitOnce(listID, list.isSuccess && list.isFetchedAfterMount);
  const notice = list.data ? (noticeOf(list.data) ?? "none") : undefined;
  const pressed = useChangedSinceShown(notice);
  if (list.isPending) {
    return <ListPending />;
  }
  if (list.isError) {
    return <FocusedState title={t("lists.page")} sentence={t("lists.gone")} />;
  }
  return (
    <div className="wrap lists-page">
      {/* Archive list, Restore and "Look after it" each remove the button that
          was pressed by changing the notice, so a fresh head takes the focus
          that would otherwise fall to <body>. */}
      <ListHead key={notice} list={list.data} arrived={pressed} />
      <ListNotices list={list.data} />
      <ListMembers list={list.data} />
      <ListHistoryPanel list={list.data} />
    </div>
  );
}

/**
 * Whether `value` differs from the one this page last committed: a verb's
 * doing, where the first value the page shows is its arrival.
 */
function useChangedSinceShown(value: string | undefined): boolean {
  const shown = useRef(value);
  useEffect(() => {
    shown.current = value;
  }, [value]);
  return shown.current !== undefined && value !== shown.current;
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

function ListHead({
  list,
  arrived,
}: Readonly<{ list: List; arrived: boolean }>) {
  const name = useArrivalFocus<HTMLHeadingElement>(arrived);
  const exporting = useListExport(list);
  return (
    <header className="lists-head">
      <div className="lists-head-row">
        <div className="lists-head-title">
          <Heading size="xlarge" ref={name} tabIndex={-1}>
            {list.name}
          </Heading>
          <ListKindBadge list={list} />
          {/* The archived notice wins over a health notice, so the health
              is said here instead. */}
          {noticeOf(list) === "archived" && <ListHealthBadge list={list} />}
        </div>
        <ListHeadActions list={list} exporting={exporting} />
      </div>
      <ErrorLine error={exporting.run.error} />
      {list.purpose && <p className="lists-note">{list.purpose}</p>}
      <ListFilterLine list={list} />
      <ListSinceVisit list={list} />
      <div className="lists-head-facts">
        <ListFacts list={list} />
      </div>
    </header>
  );
}

/**
 * The verb any reader can use leads, then the steward's, then the menu of
 * rarer ones. Export CSV arrives once the members are read, so the group holds
 * its place while they load: folded, it is a row of its own (lists.css).
 */
function ListHeadActions({
  list,
  exporting,
}: Readonly<{ list: List; exporting: ReturnType<typeof useListExport> }>) {
  const t = useT();
  const changeable = list.can_edit && !list.archived_at;
  if (
    !exporting.offered &&
    !exporting.awaited &&
    !mayEditFilter(list) &&
    !changeable
  ) {
    return null;
  }
  return (
    <div className="lists-head-actions">
      {exporting.offered && (
        <Button
          variant="ghost"
          pending={exporting.run.isPending}
          onClick={() =>
            exporting.run.mutate({
              listId: list.id,
              recordType: list.entity_type,
            })
          }
        >
          {t("filters.exportCsv")}
        </Button>
      )}
      <EditFilterAction list={list} />
      {changeable && (
        <OverflowMenu label={t("filters.library.rowMore", { name: list.name })}>
          <ListSettingsAction list={list} />
          <ArchiveListAction list={list} />
        </OverflowMenu>
      )}
    </div>
  );
}

/**
 * What the list is, laid out as the record heads lay out theirs. While the
 * ownerless notice says nobody looks after the list, it names no steward here.
 */
function ListFacts({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const audienceOf = useListAudienceLabel();
  const exports = (list.dependencies ?? []).filter(
    (use) => use.kind === "export",
  );
  const latest = exports.reduce<(typeof exports)[number] | undefined>(
    (last, use) =>
      last === undefined ||
      Date.parse(use.occurred_at) > Date.parse(last.occurred_at)
        ? use
        : last,
    undefined,
  );
  return (
    <RecordFacts>
      <Fact label={t("lists.col.recordType")}>
        <ListRecordsCount list={list} />
      </Fact>
      <Fact label={t("lists.sharingLabel")}>{audienceOf(list)}</Fact>
      {noticeOf(list) !== "ownerless" && (
        <Fact label={t("lists.fact.steward")}>
          {list.steward_name ?? t("lists.noSteward")}
        </Fact>
      )}
      {latest && (
        <Fact label={t("lists.fact.exported")}>
          {plural("lists.head.exported", exports.length, {
            count: formatNumber(exports.length, locale),
            when: formatDateTime(latest.occurred_at, locale, viewerZone()),
          })}
        </Fact>
      )}
    </RecordFacts>
  );
}

/**
 * What a Live List selects, as one sentence, once the vocabulary names its
 * fields. A Shortlist has no filter, and a definition this page cannot read
 * says nothing rather than something wrong.
 */
function ListFilterLine({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const words = useSentenceWords();
  const live = list.list_type === "dynamic";
  const vocabulary = useFilterVocabulary(list.entity_type, live);
  // Decoding mints a fresh id for every node, so it runs once per definition.
  const tree = useMemo(
    () => (live ? decode(list.definition) : null),
    [live, list.definition],
  );
  const fields = vocabulary.data?.fields;
  if (tree === null || fields === undefined) {
    return null;
  }
  return (
    <p className="t-caption">
      {t("lists.filterLine", {
        sentence: filterSentence(tree, fields, words),
      })}
    </p>
  );
}

/** What a Live List gained and lost since the reader's last visit. */
function ListSinceVisit({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const { locale } = useLocale();
  if (list.list_type !== "dynamic") {
    return null;
  }
  const type = list.entity_type;
  const pulse = list.since_last_visit;
  if (list.changes_since_visit) {
    return (
      <ListChangeSummary
        summary={list.changes_since_visit}
        onOpen={
          isMemberSource(type)
            ? (id) => navigate({ screen: MEMBER_SOURCES[type].screen, id })
            : undefined
        }
      />
    );
  }
  if (!pulse || pulse.entered + pulse.left === 0) {
    return null;
  }
  return (
    <p className="t-caption">
      {t("lists.head.pulse", {
        entered: formatNumber(pulse.entered, locale),
        left: formatNumber(pulse.left, locale),
      })}
    </p>
  );
}

type ListNotice = "archived" | "invalid" | "retired_field" | "ownerless";

/** The one standing state the list's notice speaks for, most pressing first. */
function noticeOf(list: List): ListNotice | null {
  if (list.archived_at) {
    return "archived";
  }
  if (
    list.health === "invalid" ||
    list.health === "retired_field" ||
    list.health === "ownerless"
  ) {
    return list.health;
  }
  return null;
}

/**
 * What the list says about itself: archived, broken, looked after by nobody,
 * or filtering on a field that was retired. Every reader reads why; only those
 * allowed to act see the verb that fixes it.
 */
function ListNotices({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const plural = usePlural();
  const me = useMe();
  const update = useUpdateList();
  const archive = useArchiveList();
  const notice = noticeOf(list);
  if (notice === "archived") {
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
        <p>{t("lists.archived.body")}</p>
        <ErrorLine error={archive.error} />
      </Callout>
    );
  }
  if (notice === "invalid") {
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
  if (notice === "retired_field") {
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
  if (notice !== "ownerless") {
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
      <p>{t("lists.ownerless.body")}</p>
      <ErrorLine error={update.error} />
    </Callout>
  );
}

function ListMembers({ list }: Readonly<{ list: List }>) {
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
    <MembersPanel
      list={list}
      source={list.entity_type}
      onOpen={(row) => navigate({ screen: source.screen, id: row.id })}
      columns={columns}
    />
  );
}
