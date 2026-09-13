// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { navigate } from "../app/router";
import { Badge, Button, Disclosure } from "../design-system/atoms";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { isBriefUpdate, waitingRows } from "./brief.sentence";
import { worklistLaneHref } from "./worklist.header";
import { hasPane } from "./worklist.pane";
import {
  type Worklist,
  type WorklistItem,
  worklistKey,
} from "./worklist.queries";
import { WorklistRow } from "./worklist.row";

import "./worklist.css";
import "./brief.feed.css";

export function BriefFeed({
  day,
  state,
  changed,
  refreshFailed = false,
  onRetry,
  onContext,
}: Readonly<{
  day: Worklist | undefined;
  state: SectionState;
  changed?: Readonly<{ count: number; href: string }>;
  refreshFailed?: boolean;
  onRetry?: () => void;
  onContext?: (item: WorklistItem) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const client = useQueryClient();
  const rows = waitingRows(day);
  const partial = Boolean(
    !day?.focus ||
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
        footer={day ? <AgendaFoot day={day} /> : undefined}
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
          <AgendaRows
            rows={rows}
            onOpenEmail={setOpenEmail}
            onContext={onContext}
            focus
          />
        </SurfaceState>
      </Panel>
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
        onReplySent={() =>
          void client.invalidateQueries({ queryKey: worklistKey })
        }
      />
    </section>
  );
}

function AgendaFoot({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const hidden = day.focus?.urgent_remaining ?? 0;
  return (
    <>
      <a className="entity-link" href={worklistLaneHref("all", day.scope)}>
        {t("brief.feed.fullWorklist")}
      </a>
      {hidden > 0 && (
        <a className="entity-link" href={worklistLaneHref("urgent", day.scope)}>
          {plural("brief.focus.urgentRemaining", hidden, {
            count: formatNumber(hidden, locale),
          })}
        </a>
      )}
      {day.focus && day.focus.total > day.focus.items.length && (
        <span className="t-caption">
          {t("brief.focus.remaining", {
            count: formatNumber(
              day.focus.total - day.focus.items.length,
              locale,
            ),
          })}
        </span>
      )}
    </>
  );
}

function AgendaRows({
  rows,
  onOpenEmail,
  focus = false,
  onContext,
}: Readonly<{
  rows: readonly WorklistItem[];
  onOpenEmail: (id: string) => void;
  focus?: boolean;
  onContext?: (item: WorklistItem) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const routine = rows.filter(
    (item) =>
      !item.overdue &&
      item.level === 6 &&
      (item.source === "notice_case" || item.source === "dsr"),
  );
  const grouped = new Set(!focus && routine.length > 1 ? routine : []);
  const draw = (item: WorklistItem) => (
    <li key={`${item.source}-${item.id}`}>
      <Panel
        footer={
          focus && onContext && (hasPane(item) || item.source === "task") ? (
            <Button small variant="ghost" onClick={() => onContext(item)}>
              {t("brief.focus.context")}
            </Button>
          ) : undefined
        }
      >
        <WorklistRow
          item={item}
          density="compact"
          owner=""
          onOpenEmail={onOpenEmail}
          onReview={() =>
            navigate(
              { screen: "worklist" },
              new Map([["filter", item.category]]),
            )
          }
        />
      </Panel>
    </li>
  );
  return (
    <ol
      className={focus ? "brief-feed-list brief-focus-grid" : "brief-feed-list"}
    >
      {rows.filter((item) => !grouped.has(item)).map(draw)}
      {grouped.size > 0 && (
        <li>
          <Disclosure
            summary={t("brief.feed.routine", {
              count: formatNumber(routine.length, locale),
            })}
          >
            <ol
              className={
                focus ? "brief-feed-list brief-focus-grid" : "brief-feed-list"
              }
            >
              {routine.map(draw)}
            </ol>
          </Disclosure>
        </li>
      )}
    </ol>
  );
}

export function BriefUpdates({ day }: Readonly<{ day: Worklist | undefined }>) {
  const t = useT();
  const rows = (day?.queue ?? []).filter(isBriefUpdate);
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const client = useQueryClient();
  if (rows.length === 0) return null;
  return (
    <Panel title={t("brief.updates.title")}>
      <AgendaRows rows={rows} onOpenEmail={setOpenEmail} />
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
        onReplySent={() =>
          void client.invalidateQueries({ queryKey: worklistKey })
        }
      />
    </Panel>
  );
}
