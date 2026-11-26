// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { Box, Check, Copy } from "lucide-react";
import { api, FIRST_PAGE } from "../../api/client";
import { NAV } from "../../app/nav";
import { EntityRef } from "../../components/entityref";
import { SeatAvatar } from "../../components/seatavatar";
import { Avatar, Badge, EmptyState } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { useSettledValue } from "../../design-system/debouncedsearch";
import { IconAction } from "../../design-system/iconaction";
import { Panel, PanelBody } from "../../design-system/panel";
import { LoadMoreButton, QueryStates } from "../../design-system/querystates";
import { Chip } from "../../design-system/readings";
import { formatDateTime } from "../../format/format";
import { useNow } from "../../format/now";
import { formatRelativeTime } from "../../format/relativetime";
import { useLocale, useT } from "../../i18n";
import { actorAttribution, qualifierPhrase } from "../../lib/auditactor";
import { useCan } from "../../lib/capability";
import { ENTITY, isEntityKind } from "../../lib/entity";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { useUrlParams } from "../../lib/urlstate";
import { useViewerZone } from "../../lib/viewerzone";
import { ACTOR_ICON, actionLabel, entityLabel } from "./audit";
import { AuditDetail } from "./settings-audit.detail";
import { AuditLogFilterBar } from "./settings-audit.dials";
import {
  ACTION_TONE,
  type AuditLogEntry,
  type AuditLogFilters,
  auditLogFiltersOf,
  auditLogQueryParams,
  idTail,
  isNarrowed,
  isRangeReversed,
  withAuditLogFilters,
} from "./settings-audit.format";
import "./settings-audit.css";

type Viewer = Readonly<{ id?: string; name?: string; photo?: string }>;

const RELATIVE_TICK_MS = 60_000;

export function AuditLogCard() {
  const user = useMe().data?.user;
  const canSee = useCan("audit_log", "read");
  const [params, setParams] = useUrlParams();
  const filters = auditLogFiltersOf(params);
  const asked = useSettledValue(filters);
  const query = useAuditLog(asked, canSee);
  const entries: AuditLogEntry[] =
    query.data?.pages.flatMap((page) => page.data) ?? [];
  return (
    <>
      {canSee && (
        <AuditLogFilterBar
          filters={filters}
          entries={entries}
          onChange={(next) => setParams(withAuditLogFilters(params, next))}
        />
      )}
      <Panel>
        <AuditLogEntries
          query={query}
          entries={entries}
          filters={asked}
          viewer={{
            id: user?.id,
            name: user?.display_name,
            photo: user?.avatar_url,
          }}
        />
      </Panel>
    </>
  );
}

// A reversed window is said at its field and never asked of the server.
function useAuditLog(filters: AuditLogFilters, canSee: boolean) {
  return useInfiniteQuery({
    queryKey: ["audit-log", filters],
    enabled: canSee && !isRangeReversed(filters),
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
}

function AuditLogEntries({
  query,
  entries,
  filters,
  viewer,
}: Readonly<{
  query: ReturnType<typeof useAuditLog>;
  entries: readonly AuditLogEntry[];
  filters: AuditLogFilters;
  viewer: Viewer;
}>) {
  const t = useT();
  // Withheld, not absent: a missing trail would read as nothing having happened.
  const canSee = useCan("audit_log", "read");

  if (!canSee) {
    return (
      <PanelBody>
        <EmptyState>{t("settings.auditAdminOnly")}</EmptyState>
      </PanelBody>
    );
  }
  if (isRangeReversed(filters)) {
    return (
      <PanelBody>
        <EmptyState>{t("settings.auditRangeReversed")}</EmptyState>
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
          {actionLabel(entry.action, t)}
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
            action: actionLabel(entry.action, t),
            entity: entityLabel(entry.entity_type, t),
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
  const readerZone = useViewerZone();
  return (
    <time
      className="auditlog-time"
      dateTime={at}
      title={formatDateTime(at, locale, readerZone)}
    >
      {formatRelativeTime(at, locale, new Date(now))}
    </time>
  );
}

const HUMAN_ID = /^(?:human|buyer):/;

// Only a member's seat has a picture; a buyer has none.
const SEAT_ID = /^human:/;

function AuditActor({
  entry,
  viewer,
}: Readonly<{ entry: AuditLogEntry; viewer: Viewer }>) {
  const t = useT();
  const who = actorAttribution(entry, viewer.id);
  const label =
    who.name ?? (who.labelKey ? t(who.labelKey) : (who.identifier ?? ""));
  const qualifier = qualifierPhrase(who, t);
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
    return (
      <Avatar name={viewer.name} identity={viewer.id} photo={viewer.photo} />
    );
  }
  if (name && SEAT_ID.test(entry.actor_id)) {
    return <SeatAvatar id={entry.actor_id.replace(SEAT_ID, "")} name={name} />;
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
  const t = useT();
  const label = entry.entity_label?.trim();
  return (
    <div className="auditlog-target-cell">
      <Chip dense icon={entityIcon(entry.entity_type)}>
        {entityLabel(entry.entity_type, t)}
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
