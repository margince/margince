// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { navigate } from "../app/router";
import { Badge, Button, Disclosure } from "../design-system/atoms";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { waitingRows } from "./brief.sentence";
import { worklistLaneHref } from "./worklist.header";
import type { Worklist, WorklistItem } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";

import "./worklist.css";
import "./brief.feed.css";

export function BriefFeed({
  day,
  state,
  changed,
  onMore,
  loadingMore = false,
  moreFailed = false,
  refreshFailed = false,
  onRetry,
}: Readonly<{
  day: Worklist | undefined;
  state: SectionState;
  changed?: Readonly<{ count: number; href: string }>;
  onMore?: () => void;
  loadingMore?: boolean;
  moreFailed?: boolean;
  refreshFailed?: boolean;
  onRetry?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const rows = waitingRows(day);
  const partial = Boolean(
    day?.next_cursor ||
      day?.readings?.more_available ||
      day?.sources_unavailable?.length,
  );
  return (
    <section id="brief-today">
      <Panel
        title={t(
          day?.scope === "team" ? "brief.feed.teamTitle" : "brief.feed.title",
        )}
        sub={
          day?.summary
            ? plural("brief.feed.visible", rows.length, {
                count: formatNumber(rows.length, locale),
              })
            : undefined
        }
        titleAction={
          changed ? (
            <a className="entity-link" href={changed.href}>
              <Badge>
                {plural("brief.feed.changedBadge", changed.count, {
                  count: formatNumber(changed.count, locale),
                })}
              </Badge>
            </a>
          ) : undefined
        }
        footer={
          day ? (
            <AgendaFoot
              day={day}
              onMore={onMore}
              loadingMore={loadingMore}
              moreFailed={moreFailed}
            />
          ) : undefined
        }
      >
        {refreshFailed && (
          <p role="alert">
            {t("brief.feed.refreshFailed")}{" "}
            <Button variant="ghost" onClick={onRetry}>
              {t("brief.coverage.retry")}
            </Button>
          </p>
        )}
        <SurfaceState
          state={
            state !== "ready"
              ? state
              : rows.length > 0 || partial
                ? "ready"
                : "empty"
          }
          emptyLabel={t("brief.feed.clear")}
          loadingLabel={t("brief.feed.loading")}
        >
          {rows.length === 0 && partial && (
            <p className="t-caption">{t("brief.feed.incomplete")}</p>
          )}
          <AgendaRows rows={rows} onOpenEmail={setOpenEmail} />
        </SurfaceState>
      </Panel>
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
      />
    </section>
  );
}

function AgendaFoot({
  day,
  onMore,
  loadingMore,
  moreFailed,
}: Readonly<{
  day: Worklist;
  onMore?: () => void;
  loadingMore: boolean;
  moreFailed: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = day.queue;
  const plural = usePlural();
  const known = rows.every((item) => item.urgent !== undefined);
  const loaded = rows.reduce(
    (sum, item) => sum + (item.urgent ? (item.batch?.count ?? 1) : 0),
    0,
  );
  const hidden = known ? Math.max(0, day.summary.urgent - loaded) : 0;
  return (
    <>
      {day.next_cursor && onMore && (
        <Button small pending={loadingMore} onClick={onMore}>
          {t(moreFailed ? "brief.feed.retryMore" : "brief.feed.showMore")}
        </Button>
      )}
      <a className="entity-link" href={worklistLaneHref("all", day.scope)}>
        {t("brief.feed.fullWorklist")}
      </a>
      {day.next_cursor && (
        <span className="t-caption">
          {hidden > 0
            ? plural("brief.feed.remainingUrgent", hidden, {
                count: formatNumber(hidden, locale),
              })
            : t("brief.feed.moreUrgentPossible")}
        </span>
      )}
    </>
  );
}

function AgendaRows({
  rows,
  onOpenEmail,
}: Readonly<{
  rows: readonly WorklistItem[];
  onOpenEmail: (id: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const routine = rows.filter(
    (item) =>
      !item.overdue &&
      item.level === 6 &&
      (item.source === "notice_case" || item.source === "dsr"),
  );
  const grouped = new Set(routine.length > 1 ? routine : []);
  const draw = (item: WorklistItem) => (
    <li key={`${item.source}-${item.id}`}>
      <WorklistRow
        item={item}
        density="compact"
        owner=""
        onOpenEmail={onOpenEmail}
        onReview={() =>
          navigate({ screen: "worklist" }, new Map([["filter", item.category]]))
        }
      />
    </li>
  );
  return (
    <ol className="brief-feed-list">
      {rows.filter((item) => !grouped.has(item)).map(draw)}
      {grouped.size > 0 && (
        <li>
          <Disclosure
            summary={t("brief.feed.routine", {
              count: formatNumber(routine.length, locale),
            })}
          >
            <ol className="brief-feed-list">{routine.map(draw)}</ol>
          </Disclosure>
        </li>
      )}
    </ol>
  );
}
