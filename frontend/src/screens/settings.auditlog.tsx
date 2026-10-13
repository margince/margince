import { useInfiniteQuery } from "@tanstack/react-query";
import { PanelRightOpen } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { isEntityKind } from "../app/entity";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button, EmptyState, Skeleton } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateTime } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { ActorTag } from "./audit";
import {
  LoadMoreButton,
  problemMessageOf,
  throwProblem,
  useMe,
} from "./common";
import { EntityRef } from "./entityref";
import { SEARCH_DEBOUNCE_MS } from "./listquery";
import { AuditLogDetail } from "./settings.auditlog.detail";
import {
  type AuditLogFilters,
  AuditLogToolbar,
  UNFILTERED_AUDIT_LOG,
} from "./settings.auditlog.toolbar";
import {
  AUDIT_ACTION_LABEL,
  AUDIT_TYPE_LABEL,
  auditActionTone,
  auditWhenRange,
  sentenceCaseToken,
  shortId,
} from "./settings.auditlog.vocab";
import "./settings.auditlog.css";

type AuditLogEntry = components["schemas"]["AuditLogEntry"];

const AUDIT_PAGE_SIZE = 20;

// Every filter is optional-if-blank, so this stays a flat spread rather than a
// chain of conditionals in the queryFn. `now` is read when the question is
// asked, so "Today" means the day the request leaves on.
function auditLogQueryParams(
  filters: AuditLogFilters,
  pageParam: string | null,
  now: Date,
  zone: string,
) {
  const { actor, entityType, entityId, batchId, action } = filters;
  return {
    limit: AUDIT_PAGE_SIZE,
    ...(pageParam ? { cursor: pageParam } : {}),
    ...(actor.trim() ? { actor: actor.trim() } : {}),
    ...(entityType ? { entity_type: entityType } : {}),
    ...(entityId.trim() ? { entity_id: entityId.trim() } : {}),
    ...(batchId.trim() ? { batch_id: batchId.trim() } : {}),
    ...(action ? { action } : {}),
    ...auditWhenRange(filters.when, filters, now, zone),
  };
}

/**
 * The filters as a QUESTION, settled, which is a different thing from the
 * filters as they are being typed. The filter object is the query key, so
 * without this every typed character was its own `GET /audit-log` and the
 * answers could land out of order.
 */
function useSettledAuditLogFilters(typed: AuditLogFilters): AuditLogFilters {
  const [settled, setSettled] = useState(typed);
  useEffect(() => {
    const timer = setTimeout(() => setSettled(typed), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [typed]);
  return settled;
}

function isFiltered(filters: AuditLogFilters): boolean {
  return Object.entries(filters).some(
    ([key, value]) =>
      value !== UNFILTERED_AUDIT_LOG[key as keyof AuditLogFilters],
  );
}

// The record a row changed: its name where EntityRef can resolve one, else the
// type in words plus enough of the id to tell two rows apart. Never the raw
// table name.
function AuditRecordCell({ entry }: Readonly<{ entry: AuditLogEntry }>) {
  const t = useT();
  const labelKey = AUDIT_TYPE_LABEL[entry.entity_type];
  const typeLabel = labelKey
    ? t(labelKey)
    : sentenceCaseToken(entry.entity_type);
  if (entry.entity_id && isEntityKind(entry.entity_type)) {
    return (
      <CellStack>
        <EntityRef kind={entry.entity_type} id={entry.entity_id} />
        <span className="t-caption">{typeLabel}</span>
      </CellStack>
    );
  }
  return (
    <CellStack>
      <span>{typeLabel}</span>
      {entry.entity_id && (
        <span className="t-caption audit-log-id">
          {shortId(entry.entity_id)}
        </span>
      )}
    </CellStack>
  );
}

function useAuditColumns(
  meUserId: string | undefined,
  onOpen: (entry: AuditLogEntry) => void,
): DataTableColumn<AuditLogEntry>[] {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return [
    {
      key: "when",
      header: t("settings.auditLog.when"),
      // The company's clock, the one record history reads on: an entry is a
      // fact in the shared book, and on the viewer's clock two operators
      // quoting one line would quote different days.
      render: (entry) => (
        <time className="audit-log-when" dateTime={entry.occurred_at}>
          {formatDateTime(entry.occurred_at, locale, recordZone)}
        </time>
      ),
    },
    {
      key: "who",
      header: t("settings.auditLog.who"),
      render: (entry) => <ActorTag entry={entry} meUserId={meUserId} />,
    },
    {
      key: "what",
      header: t("settings.auditLog.what"),
      render: (entry) => (
        <Badge tone={auditActionTone(entry.action)}>
          {t(AUDIT_ACTION_LABEL[entry.action])}
        </Badge>
      ),
    },
    {
      key: "record",
      header: t("settings.auditLog.record"),
      grow: true,
      render: (entry) => <AuditRecordCell entry={entry} />,
    },
    {
      key: "details",
      header: t("settings.auditLog.details"),
      align: "end",
      render: (entry) => (
        <Button
          iconOnly
          aria-label={t("settings.auditLog.open")}
          onClick={() => onOpen(entry)}
        >
          <PanelRightOpen aria-hidden />
        </Button>
      ),
    },
  ];
}

function AuditLogTable({
  entries,
  meUserId,
}: Readonly<{ entries: AuditLogEntry[]; meUserId?: string }>) {
  const t = useT();
  const [open, setOpen] = useState<AuditLogEntry | null>(null);
  const columns = useAuditColumns(meUserId, setOpen);
  return (
    <>
      <div className="audit-log-table">
        <DataTable
          label={t("settings.auditLog.title")}
          columns={columns}
          rows={entries}
          rowKey={(entry) => entry.id}
        />
      </div>
      {open && (
        <AuditLogDetail
          entry={open}
          meUserId={meUserId}
          onClose={() => setOpen(null)}
        />
      )}
    </>
  );
}

// The result half: the answer to whatever the toolbar currently asks. A filter
// change is a new question, since the filters ARE the query key, so it restarts
// the cursor chain instead of appending to a stale one.
function AuditLogEntries({
  filters,
  meUserId,
  onClear,
}: Readonly<{
  filters: AuditLogFilters;
  meUserId?: string;
  onClear: () => void;
}>): ReactNode {
  const t = useT();
  const zone = useRecordZone();
  // `audit_log:read`, which is what privacy.ListAuditLog asks for. The fetch is
  // disabled without it: a reader must not issue a call that can only 403.
  const canSee = useCan("audit_log", "read");
  const query = useInfiniteQuery({
    queryKey: ["audit-log", filters, zone],
    enabled: canSee,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/audit-log", {
        params: {
          query: auditLogQueryParams(filters, pageParam, new Date(), zone),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
  const entries = query.data?.pages.flatMap((page) => page.data) ?? [];
  const filtered = isFiltered(filters);

  if (!canSee) {
    // Withheld rather than absent: an absent trail would read as "nothing has
    // happened here", a different claim from "this is not yours to read".
    return <EmptyState>{t("settings.auditLog.noGrant")}</EmptyState>;
  }
  if (query.isPending) {
    return (
      <div className="audit-log-loading">
        <Skeleton width="60%" />
        <Skeleton width="90%" />
        <Skeleton width="75%" />
      </div>
    );
  }
  if (query.isError) {
    return (
      <EmptyState>
        <p>{t("common.error")}</p>
        {/* ds:ignore the cause under an EmptyState's headline */}
        <p className="audit-log-error-cause">
          {problemMessageOf(query.error, t)}
        </p>
        <Button onClick={() => query.refetch()}>{t("common.retry")}</Button>
      </EmptyState>
    );
  }
  if (entries.length === 0) {
    return filtered ? (
      <EmptyState>
        <p>{t("settings.auditLog.filteredEmpty")}</p>
        <Button onClick={onClear}>{t("settings.auditLog.clear")}</Button>
      </EmptyState>
    ) : (
      <EmptyState>{t("settings.auditLog.empty")}</EmptyState>
    );
  }
  return (
    <>
      <AuditLogCountLine
        shown={entries.length}
        more={query.hasNextPage}
        filtered={filtered}
        zone={zone}
        onClear={onClear}
      />
      <AuditLogTable entries={entries} meUserId={meUserId} />
      <div className="audit-log-more">
        <LoadMoreButton query={query} />
      </div>
    </>
  );
}

// What the table is showing, said above it: unfiltered, how it is ordered and
// which clock it reads on; narrowed, how many rows are loaded and the way back.
function AuditLogCountLine({
  shown,
  more,
  filtered,
  zone,
  onClear,
}: Readonly<{
  shown: number;
  more: boolean;
  filtered: boolean;
  zone: string;
  onClear: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  if (!filtered) {
    return (
      <p className="t-caption audit-log-countline">
        {t("settings.auditLog.countLine", { zone })}
      </p>
    );
  }
  return (
    <div className="audit-log-countline">
      <span className="t-caption">
        {plural(
          more ? "settings.auditLog.shownMore" : "settings.auditLog.shown",
          shown,
          { count: String(shown) },
        )}
      </span>
      <Button variant="link" onClick={onClear}>
        {t("settings.auditLog.clear")}
      </Button>
    </div>
  );
}

// AC-settings-16: the attributable audit view. One panel: an always-visible
// toolbar over the six shipped filters plus the bulk change id, a table of
// changes, and keyset "load more". Each row opens its before/after diff and the
// agent attribution trail in a drawer, so the flat scan stays one line a row.
export function AuditLogCard() {
  const t = useT();
  // The BARE user id, which lets ActorTag read "You"; the wire spells a human
  // actor "human:<uuid>", and ActorTag owns that difference.
  const meUserId = useMe().data?.user?.id;
  const canSee = useCan("audit_log", "read");
  const [filters, setFilters] = useState<AuditLogFilters>(UNFILTERED_AUDIT_LOG);
  // The toolbar reads what is being typed; the entries read what has settled.
  const asked = useSettledAuditLogFilters(filters);
  return (
    <Panel title={t("settings.auditLog.title")}>
      <PanelBody>
        {/* Absent, not withheld, for a reader who may not read the trail:
            dials that narrow a list they cannot see control nothing. */}
        {canSee && (
          <AuditLogToolbar
            filters={filters}
            meUserId={meUserId}
            onChange={setFilters}
          />
        )}
        <AuditLogEntries
          filters={asked}
          meUserId={meUserId}
          onClear={() => setFilters(UNFILTERED_AUDIT_LOG)}
        />
      </PanelBody>
    </Panel>
  );
}
