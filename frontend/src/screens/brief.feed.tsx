// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { navigate } from "../app/router";
import { Badge, Button } from "../design-system/atoms";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel, PanelBody } from "../design-system/panel";
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
        {/* Prose standing where the cards will stand pays the pane's inset
            itself: the grid under it carries its own, and a sentence set
            straight into the panel printed against the card's edge. */}
        {refreshFailed && (
          <PanelBody>
            <p role="alert">
              {t("brief.feed.refreshFailed")}{" "}
              <Button variant="ghost" onClick={onRetry}>
                {t("brief.coverage.retry")}
              </Button>
            </p>
          </PanelBody>
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
            <PanelBody>
              <p className="t-caption">{t("brief.feed.incomplete")}</p>
            </PanelBody>
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
  // A list of nothing is only its own padding: under the sentence saying the
  // read was incomplete it stood as a blank band the height of two gutters.
  if (rows.length === 0) return null;
  // ONE RANKED LIST, not a grid of cards. The sentence over the page says
  // "First: X. Then N more", and a grid asked the reader to re-derive that
  // order from a Z-shaped walk across four columns. Each row carries its rank
  // as a tile in the gutter, and the first row is the lead: the tile in the
  // accent, the title at the lead size — so the eye lands where the day
  // starts and runs down from there.
  //
  // The rows sit in the panel DIRECTLY. Each was a `Panel` of its own inside
  // the Focus panel, which is the one shape the design language refuses (a
  // pane inside a pane), and the footer band each card paid for one ghost
  // button was more chrome than content.
  const draw = (item: WorklistItem, at: number) => {
    const context =
      focus && onContext && (hasPane(item) || item.source === "task");
    return (
      <li
        key={`${item.source}-${item.id}`}
        className={
          focus
            ? at === 0
              ? "brief-focus-item brief-focus-lead"
              : "brief-focus-item"
            : undefined
        }
      >
        {focus && (
          // Readable, not decorative: the list carries the order for a
          // screen reader and the tile states it for everybody else — the
          // same reason the queue's own rank is text.
          <span className="brief-focus-rank t-label t-num">
            {formatNumber(at + 1, locale)}
          </span>
        )}
        <WorklistRow
          allowPin={!focus}
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
          context={
            context ? (
              <Button small variant="ghost" onClick={() => onContext(item)}>
                {t("brief.focus.context")}
              </Button>
            ) : undefined
          }
        />
      </li>
    );
  };
  return (
    <ol
      className={focus ? "brief-feed-list brief-focus-list" : "brief-feed-list"}
    >
      {rows.map(draw)}
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
