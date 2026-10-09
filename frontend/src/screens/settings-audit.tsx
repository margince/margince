// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { Box, Check, ChevronDown, Copy } from "lucide-react";
import { type MouseEvent, useId, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import { useCan } from "../app/capability";
import { ENTITY, isEntityKind } from "../app/entity";
import { NAV } from "../app/nav";
import { useRecordZone } from "../app/recordzone";
import {
  Avatar,
  Badge,
  Disclosure,
  EmptyState,
  TableScroll,
  TextInput,
} from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { useSettledValue } from "../design-system/debouncedsearch";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Chip } from "../design-system/readings";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatDateTime } from "../format/format";
import { useNow } from "../format/now";
import { formatRelativeTime } from "../format/relativetime";
import { useLocale, useT } from "../i18n";
import { ACTOR_ICON, actorAttribution, humanizeToken } from "./audit";
import { LoadMoreButton, QueryStates, throwProblem, useMe } from "./common";
import { EntityRef } from "./entityref";
import { AuditDetail } from "./settings-audit.detail";
import {
  ACTION_TONE,
  AUDIT_LOG_FILTER_FIELDS,
  type AuditLogEntry,
  type AuditLogFilters,
  auditLogQueryParams,
  idTail,
  isNarrowed,
  UNFILTERED_AUDIT_LOG,
} from "./settings-audit.format";
import "./settings-audit.css";

type Viewer = Readonly<{ id?: string; name?: string }>;

const COLUMN_COUNT = 5;
const RELATIVE_TICK_MS = 60_000;

// The filters stay in a closed disclosure: a reader arrives to read what
// happened. Each dial applies on its own, so there is nothing to submit.
export function AuditLogCard() {
  const t = useT();
  const user = useMe().data?.user;
  const canSee = useCan("audit_log", "read");
  const [filters, setFilters] = useState<AuditLogFilters>(UNFILTERED_AUDIT_LOG);
  const asked = useSettledValue(filters);
  return (
    <Panel title={t("settings.auditEntries")}>
      <PanelBody>
        <PanelIntro>{t("settings.auditSub")}</PanelIntro>
        {canSee && (
          <Disclosure summary={t("settings.auditFilters")}>
            <AuditLogFilterFields filters={filters} onChange={setFilters} />
          </Disclosure>
        )}
      </PanelBody>
      <AuditLogEntries
        filters={asked}
        viewer={{ id: user?.id, name: user?.display_name }}
      />
    </Panel>
  );
}

function AuditLogFilterFields({
  filters,
  onChange,
}: Readonly<{
  filters: AuditLogFilters;
  onChange: (next: AuditLogFilters) => void;
}>) {
  const t = useT();
  return (
    <SettingList>
      {AUDIT_LOG_FILTER_FIELDS.map((field) => (
        <SettingRow
          key={field.key}
          label={t(field.labelKey)}
          control={(control) => (
            <TextInput
              {...control}
              className="settingrow-measure"
              type={field.date ? "date" : undefined}
              value={filters[field.key]}
              onChange={(event) =>
                onChange({ ...filters, [field.key]: event.target.value })
              }
            />
          )}
        />
      ))}
    </SettingList>
  );
}

function AuditLogEntries({
  filters,
  viewer,
}: Readonly<{ filters: AuditLogFilters; viewer: Viewer }>) {
  const t = useT();
  // Withheld, not absent: a missing trail would read as nothing having happened.
  const canSee = useCan("audit_log", "read");
  const query = useInfiniteQuery({
    queryKey: ["audit-log", filters],
    enabled: canSee,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/audit-log", {
        params: { query: auditLogQueryParams(filters, pageParam) },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
  const entries: AuditLogEntry[] =
    query.data?.pages.flatMap((page) => page.data) ?? [];

  if (!canSee) {
    return (
      <PanelBody>
        <EmptyState>{t("settings.auditAdminOnly")}</EmptyState>
      </PanelBody>
    );
  }
  if (query.isPending || query.isError) {
    return (
      <PanelBody>
        <QueryStates
          query={query}
          pendingLabel={t("settings.auditLoading")}
          pendingLines={4}
        >
          {null}
        </QueryStates>
      </PanelBody>
    );
  }
  if (entries.length === 0) {
    return (
      <PanelBody>
        <EmptyState>
          {isNarrowed(filters) ? t("settings.auditNoMatch") : t("common.empty")}
        </EmptyState>
      </PanelBody>
    );
  }
  return (
    <>
      <AuditLogTable entries={entries} viewer={viewer} />
      {query.hasNextPage && (
        <div className="auditlog-more">
          <LoadMoreButton query={query} />
        </div>
      )}
    </>
  );
}

// Not DataTable: its rows cannot carry a detail row spanning every column.
// One tbody per entry keeps an entry and its detail one unit.
function AuditLogTable({
  entries,
  viewer,
}: Readonly<{ entries: readonly AuditLogEntry[]; viewer: Viewer }>) {
  const t = useT();
  const now = useNow(RELATIVE_TICK_MS);
  return (
    <TableScroll
      label={t("settings.auditTrailLabel")}
      bleed
      className="auditlog-scroll"
    >
      <table
        className="table auditlog"
        role={ROLE.table}
        aria-label={t("settings.auditTrailLabel")}
      >
        <thead role={ROLE.rowgroup}>
          <tr role={ROLE.row}>
            <th role={ROLE.columnheader}>{t("settings.auditColWhen")}</th>
            <th role={ROLE.columnheader}>{t("settings.auditActor")}</th>
            <th role={ROLE.columnheader}>{t("settings.auditAction")}</th>
            <th role={ROLE.columnheader}>{t("settings.auditColTarget")}</th>
            <th role={ROLE.columnheader}>
              <span className="sr-only">{t("settings.auditColDetail")}</span>
            </th>
          </tr>
        </thead>
        {entries.map((entry) => (
          <AuditLogRow key={entry.id} entry={entry} viewer={viewer} now={now} />
        ))}
      </table>
    </TableScroll>
  );
}

// Spelled out because a folded row is laid out as flex, which drops the
// native table role in Safari.
const ROLE = {
  table: "table",
  rowgroup: "rowgroup",
  row: "row",
  columnheader: "columnheader",
  cell: "cell",
} as const;

// A press on a control inside the row, or a drag that selected text to copy,
// is not a request to open the entry.
function pressOpensRow(event: MouseEvent<HTMLTableRowElement>): boolean {
  const target = event.target;
  if (target instanceof Element && target.closest("a, button, code")) {
    return false;
  }
  return globalThis.getSelection?.()?.isCollapsed !== false;
}

function AuditLogRow({
  entry,
  viewer,
  now,
}: Readonly<{ entry: AuditLogEntry; viewer: Viewer; now: number }>) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);
  const detailId = useId();
  const toggle = () => setExpanded((open) => !open);
  return (
    <tbody role={ROLE.rowgroup}>
      <tr
        className="auditlog-main"
        role={ROLE.row}
        onClick={(event) => pressOpensRow(event) && toggle()}
      >
        <td className="auditlog-when" role={ROLE.cell}>
          <AuditWhen at={entry.occurred_at} now={now} />
        </td>
        <td className="auditlog-actor" role={ROLE.cell}>
          <AuditActor entry={entry} viewer={viewer} />
        </td>
        <td className="auditlog-action" role={ROLE.cell}>
          <Badge tone={ACTION_TONE[entry.action] ?? "default"}>
            {humanizeToken(entry.action)}
          </Badge>
        </td>
        <td className="auditlog-target" role={ROLE.cell}>
          <AuditTarget entry={entry} />
        </td>
        <td className="auditlog-toggle" role={ROLE.cell}>
          <IconAction
            label={t("settings.auditExpand")}
            icon={<ChevronDown aria-hidden className="expander-chevron" />}
            disclosure={{ expanded, controls: detailId }}
            onClick={toggle}
          />
        </td>
      </tr>
      {expanded && (
        <tr className="auditlog-detail-row" role={ROLE.row}>
          <td colSpan={COLUMN_COUNT} id={detailId} role={ROLE.cell}>
            <AuditDetail entry={entry} />
          </td>
        </tr>
      )}
    </tbody>
  );
}

// The exact time reads on the company's clock, as the record history does, so
// two readers in two zones quote one entry on one day.
function AuditWhen({ at, now }: Readonly<{ at: string; now: number }>) {
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <time
      className="auditlog-time"
      dateTime={at}
      title={formatDateTime(at, locale, recordZone)}
    >
      {formatRelativeTime(at, locale, new Date(now))}
    </time>
  );
}

const HUMAN_ID = /^(?:human|buyer):/;

function AuditActor({
  entry,
  viewer,
}: Readonly<{ entry: AuditLogEntry; viewer: Viewer }>) {
  const t = useT();
  const who = actorAttribution(entry, viewer.id);
  const label =
    who.name ?? (who.labelKey ? t(who.labelKey) : (who.identifier ?? ""));
  const qualifier = who.qualifierName
    ? t("audit.viaNamed", { client: who.qualifierName })
    : who.qualifierKey && t(who.qualifierKey);
  const caption = [label === who.identifier ? "" : who.identifier, qualifier]
    .filter(Boolean)
    .join(" · ");
  const MachineIcon =
    entry.actor_type === "human" ? null : ACTOR_ICON[entry.actor_type];
  return (
    <span className="auditlog-who">
      <ActorFace
        entry={entry}
        viewer={viewer}
        name={who.name}
        you={who.labelKey === "audit.you"}
      />
      <CellStack>
        <span>{label}</span>
        {caption && (
          <span className="t-caption auditlog-who-via">
            {MachineIcon && <MachineIcon aria-hidden />}
            {caption}
          </span>
        )}
      </CellStack>
    </span>
  );
}

// A monogram only for a human the row names; a machine with nobody behind it
// shows its kind, never a monogram of its id.
function ActorFace({
  entry,
  viewer,
  name,
  you,
}: Readonly<{
  entry: AuditLogEntry;
  viewer: Viewer;
  name?: string;
  you: boolean;
}>) {
  if (you && viewer.name && viewer.id) {
    return <Avatar name={viewer.name} identity={viewer.id} />;
  }
  if (name) {
    const identity = HUMAN_ID.test(entry.actor_id)
      ? entry.actor_id.replace(HUMAN_ID, "")
      : (entry.on_behalf_of ?? name);
    return <Avatar name={name} identity={identity} />;
  }
  const Icon = ACTOR_ICON[entry.actor_type];
  return (
    <span className="auditlog-face">
      <Icon aria-hidden />
    </span>
  );
}

const NAV_ICON = new Map(NAV.map((item) => [item.screen, item.icon]));

function entityIcon(entityType: string) {
  if (!isEntityKind(entityType)) {
    return Box;
  }
  return NAV_ICON.get(ENTITY[entityType].route("").screen) ?? Box;
}

function AuditTarget({ entry }: Readonly<{ entry: AuditLogEntry }>) {
  const label = entry.entity_label?.trim();
  return (
    <div className="auditlog-target-cell">
      <Chip dense icon={entityIcon(entry.entity_type)}>
        {humanizeToken(entry.entity_type)}
      </Chip>
      {label && isEntityKind(entry.entity_type) && (
        <EntityRef kind={entry.entity_type} id={entry.entity_id} name={label} />
      )}
      {label && !isEntityKind(entry.entity_type) && <span>{label}</span>}
      {!label && <ShortId id={entry.entity_id} />}
    </div>
  );
}

function ShortId({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const clip = useClipboardCopy(id, {
    copy: t("settings.auditCopyId"),
    copied: t("settings.auditIdCopied"),
    remedy: t("settings.auditCopyRemedy"),
  });
  return (
    <>
      <span className="auditlog-shortid">
        <code className="auditlog-id" title={id}>
          {idTail(id)}
        </code>
        <IconAction
          inline
          label={clip.label}
          icon={clip.copied ? <Check aria-hidden /> : <Copy aria-hidden />}
          onClick={clip.copy}
        />
      </span>
      {clip.notice}
    </>
  );
}
