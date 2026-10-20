// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Badge, Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { navigate } from "../../lib/router";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { worklistLaneHref } from "../worklist/worklist.header";
import {
  type Worklist,
  type WorklistItem,
  worklistKey,
} from "../worklist/worklist.queries";
import { WorklistRow } from "../worklist/worklist.row";
import { identity, Triage } from "./brief.focus";
import { laneUnread } from "./brief.readings.honesty";
import { isBriefUpdate, waitingRows } from "./brief.sentence";

import "../worklist/worklist.css";
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
  // Which row is in hand, by identity rather than index. The queue is re-read
  // every minute, so an index would hand over the row that moved up. A chosen
  // row that has gone falls back to the day's first.
  const [chosen, setChosen] = useState<string | null>(null);
  const client = useQueryClient();
  const rows = waitingRows(day);
  const at = Math.max(
    0,
    rows.findIndex((item) => identity(item) === chosen),
  );
  const lead = rows[at];
  const partial = Boolean(
    !day?.focus || day.readings?.more_available || laneUnread(day),
  );
  return (
    <section id="brief-today">
      <Panel
        // Indigo, the band a record's own "what needs you today" pane wears.
        // These rows are the agent's ranking and preparation, and indigo is
        // the product's claim about who did that.
        tone="ai"
        title={t(
          day?.scope === "team" ? "brief.feed.teamTitle" : "brief.feed.title",
        )}
        titleAction={
          <span className="brief-focus-actions">
            {/* The panel offers the agent's verbs, so it wears the badge every
                panel offering an AI verb wears. */}
            <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
            {changed && (
              <a className="entity-link" href={changed.href}>
                <Badge>
                  {plural("brief.feed.changedBadge", changed.count, {
                    count: formatNumber(changed.count, locale),
                  })}
                </Badge>
              </a>
            )}
          </span>
        }
        footer={day && hasFoot(day) ? <AgendaFoot day={day} /> : undefined}
      >
        {/* Prose where the rows will stand pays the pane's inset itself. The
            triage carries its own, and bare prose prints against the edge. */}
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
              <p>{t("brief.feed.incomplete")}</p>
            </PanelBody>
          )}
          {lead && (
            <Triage
              rows={rows}
              at={at}
              lead={lead}
              onChoose={(item) => setChosen(identity(item))}
              onOpenEmail={setOpenEmail}
              onContext={onContext}
            />
          )}
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

/**
 * The focus, one row at a time. The row in hand is drawn whole, and beside it
 * sits the ranked queue it was taken from. Each queue row is a press that puts
 * it in hand.
 *
 * A reader clears a morning by answering one thing and moving to the next.
 * On the left is the row at full density: its kind, message, reasons, the cost
 * of doing nothing, and every verb. On the right is the order the day put the
 * rest in. The sentence over the page says "First: X. Then N more", and this
 * is that sentence drawn.
 */
function hasFoot(day: Worklist): boolean {
  return (
    (day.focus?.urgent_remaining ?? 0) > 0 ||
    (day.focus !== undefined && day.focus.total > day.focus.items.length)
  );
}

function AgendaFoot({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const hidden = day.focus?.urgent_remaining ?? 0;
  return (
    <>
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

/** The plain feed: every row at the queue's own density, in the day's order. */
function AgendaRows({
  rows,
  onOpenEmail,
}: Readonly<{
  rows: readonly WorklistItem[];
  onOpenEmail: (id: string) => void;
}>) {
  // An empty list is only its own padding, a blank band under the incomplete-read sentence.
  if (rows.length === 0) return null;
  return (
    <ol className="brief-feed-list">
      {rows.map((item) => (
        <li key={identity(item)}>
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
