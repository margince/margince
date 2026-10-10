// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { Box, Check, Copy } from "lucide-react";
import { useState } from "react";
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
  TextInput,
} from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
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
import { LoadMoreButton, QueryStates, unwrap, useMe } from "./common";
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
      return unwrap(
        await api.GET("/audit-log", {
          params: { query: auditLogQueryParams(filters, pageParam) },
        }),
      );
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

function AuditLogTable({
  entries,
  viewer,
}: Readonly<{ entries: readonly AuditLogEntry[]; viewer: Viewer }>) {
  const t = useT();
  const now = useNow(RELATIVE_TICK_MS);
  const columns: DataTableColumn<AuditLogEntry>[] = [
    {
      key: "when",
      header: t("settings.auditColWhen"),
      render: (entry) => <AuditWhen at={entry.occurred_at} now={now} />,
    },
    {
      key: "actor",
      header: t("settings.auditActor"),
      render: (entry) => <AuditActor entry={entry} viewer={viewer} />,
    },
    {
      key: "action",
      header: t("settings.auditAction"),
      render: (entry) => (
        <Badge tone={ACTION_TONE[entry.action] ?? "default"}>
          {humanizeToken(entry.action)}
        </Badge>
      ),
    },
    {
      key: "target",
      header: t("settings.auditColTarget"),
      fold: "title",
      render: (entry) => <AuditTarget entry={entry} />,
    },
  ];
  return (
    <DataTable
      bleed
      fold
      label={t("settings.auditTrailLabel")}
      columns={columns}
      rows={[...entries]}
      rowKey={(entry) => entry.id}
      detail={{
        header: t("settings.auditColDetail"),
        toggleLabel: (entry) =>
          t("settings.auditExpandEntry", {
            action: entry.action,
            entity: entry.entity_type,
          }),
        render: (entry) => <AuditDetail entry={entry} />,
      }}
    />
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

// The contract requires the id, but a server from another version may omit it.
function ShortId({ id }: Readonly<{ id: string | null | undefined }>) {
  const t = useT();
  const full = id ?? "";
  const clip = useClipboardCopy(full, {
    copy: t("settings.auditCopyId"),
    copied: t("settings.auditIdCopied"),
    remedy: t("settings.auditCopyRemedy"),
  });
  const tail = idTail(full);
  if (!tail) {
    return <span className="t-caption">{t("settings.auditNoValue")}</span>;
  }
  return (
    <>
      <span className="auditlog-shortid">
        <code className="auditlog-id" title={full}>
          {tail}
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
