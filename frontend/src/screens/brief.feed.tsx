// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { navigate } from "../app/router";
import { Badge, Button } from "../design-system/atoms";
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
import { decidable } from "./worklist.rowdecision";

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
  const titleId = useId();
  const rows = waitingRows(day);
  const partial = Boolean(
    !day?.focus ||
      day?.readings?.more_available ||
      day?.sources_unavailable?.length,
  );
  return (
    <section id="brief-today" className="brief-focus" aria-labelledby={titleId}>
      {/* No panel band. The greeting above already heads the page, and a
          card around six cards was a box around boxes: the section has an
          eyebrow, the lead is the one elevated surface, and the rest is a
          list on the page's own ground. */}
      <header className="brief-focus-head">
        <h2 id={titleId} className="eyebrow">
          {t(
            day?.scope === "team" ? "brief.feed.teamTitle" : "brief.feed.title",
          )}
        </h2>
        {day?.summary && (
          <span className="t-caption">
            {plural("brief.feed.visible", rows.length, {
              count: formatNumber(rows.length, locale),
            })}
          </span>
        )}
        {changed && (
          <a className="entity-link" href={changed.href}>
            <Badge>
              {plural("brief.feed.changedBadge", changed.count, {
                count: formatNumber(changed.count, locale),
              })}
            </Badge>
          </a>
        )}
      </header>
      {refreshFailed && (
        <p className="t-caption brief-focus-note" role="alert">
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
          <p className="t-caption brief-focus-note">
            {t("brief.feed.incomplete")}
          </p>
        )}
        <AgendaRows
          rows={rows}
          onOpenEmail={setOpenEmail}
          onContext={onContext}
          focus
        />
      </SurfaceState>
      {day && (
        <footer className="brief-focus-foot">
          <AgendaFoot day={day} />
        </footer>
      )}
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
  // A list of nothing is only its own padding: under the sentence saying the
  // read was incomplete it stood as a blank band the height of two gutters.
  if (rows.length === 0) return null;
  const door = (item: WorklistItem) =>
    focus && onContext && (hasPane(item) || item.source === "task")
      ? () => onContext(item)
      : undefined;
  const review = (item: WorklistItem) => () =>
    navigate({ screen: "worklist" }, new Map([["filter", item.category]]));
  // A DECISION IS STAGED, NOT DONE: the dashed indigo edge is the design
  // system's one mark for a value an agent proposed and nobody has accepted
  // yet, and it goes solid when the reader decides. Nothing else on a row is
  // coloured for being a row.
  const staged = (item: WorklistItem) =>
    focus && decidable(item) ? "brief-focus-staged" : undefined;
  if (!focus) {
    return (
      <ol className="brief-feed-list">
        {rows.map((item) => (
          <li key={`${item.source}-${item.id}`}>
            <Panel>
              <WorklistRow
                item={item}
                density="compact"
                owner=""
                onOpenEmail={onOpenEmail}
                onReview={review(item)}
              />
            </Panel>
          </li>
        ))}
      </ol>
    );
  }
  // THE LEAD, THEN THE ORDER. The first row is the day's answer to "what
  // first", so it is drawn open on the one elevated surface, its evidence and
  // its reason standing where a card has the height for them. The rows under
  // it are one line each with their rank in the margin: the ranking is the
  // page's structure, and six equal boxes hid it.
  const [lead, ...rest] = rows;
  return (
    <ol className="brief-focus-list">
      <li key={`${lead.source}-${lead.id}`} className="brief-focus-lead">
        <Panel className={staged(lead)}>
          <WorklistRow
            allowPin={false}
            item={lead}
            density="compact"
            card
            hero
            onOpen={door(lead)}
            owner=""
            onOpenEmail={onOpenEmail}
            onReview={review(lead)}
          />
        </Panel>
      </li>
      {rest.map((item) => (
        <li key={`${item.source}-${item.id}`} className={staged(item)}>
          <WorklistRow
            allowPin={false}
            item={item}
            density="compact"
            onOpen={door(item)}
            owner=""
            onOpenEmail={onOpenEmail}
            onReview={review(item)}
          />
        </li>
      ))}
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
