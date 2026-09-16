// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { useState } from "react";
import { navigate } from "../app/router";
import { Badge, Button } from "../design-system/atoms";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel, PanelBody } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { identity, Triage } from "./brief.focus";
import { isBriefUpdate, waitingRows } from "./brief.sentence";
import { worklistLaneHref } from "./worklist.header";
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
  // WHICH ROW IS IN HAND, by identity rather than by index: the queue is
  // re-read every minute and a row that was answered leaves it, so an index
  // would silently hand the reader the row that moved up into its place. A
  // chosen row that has gone falls back to the day's own first — the row the
  // page opens on, and the one the sentence over it names.
  const [chosen, setChosen] = useState<string | null>(null);
  const client = useQueryClient();
  const rows = waitingRows(day);
  const at = Math.max(
    0,
    rows.findIndex((item) => identity(item) === chosen),
  );
  const lead = rows[at];
  const partial = Boolean(
    !day?.focus ||
      day?.readings?.more_available ||
      day?.sources_unavailable?.length,
  );
  return (
    <section id="brief-today">
      <Panel
        // INDIGO, the same band a record's own "what needs you today" pane
        // wears: these rows are the agent's reading of the day — what it
        // ranked and what it prepared — and indigo is the one claim the
        // product makes about who did that. The accent would say "the page's
        // lead", which is true and already said by where the panel sits.
        tone="ai"
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
          <span className="brief-focus-actions">
            {/* The panel offers the agent's verbs — the reply it drafted, the
                step it prepared — so it wears the badge every panel that
                offers an AI verb wears: what is OFFERED, beside the tone that
                says who wrote the body. */}
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
            {/* The way into the whole queue, on the panel's own head: it is
                the panel's one destination, and at the foot of a list of
                verbs it read as a caption. */}
            {day && (
              <a
                className="btn btn-sm brief-focus-open"
                href={worklistLaneHref("all", day.scope)}
              >
                {t("brief.feed.fullWorklist")}
                <ArrowRight size={14} aria-hidden="true" />
              </a>
            )}
          </span>
        }
        footer={day && hasFoot(day) ? <AgendaFoot day={day} /> : undefined}
      >
        {/* Prose standing where the rows will stand pays the pane's inset
            itself: the triage under it carries its own, and a sentence set
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
 * The focus, ONE ROW AT A TIME: the row in hand drawn whole, and beside it
 * the ranked queue it was taken from, each row a press that puts it in hand.
 *
 * A reader clears a morning by answering one thing and moving to the next,
 * and the page is shaped like that act: the work on the left at the row's
 * full density — its kind, the message, its reasons, what doing nothing
 * costs, and every verb that answers it — and on the right the order the
 * day put the rest in, so the reader always sees where they are in it. The
 * sentence over the page says "First: X. Then N more", and this is that
 * sentence drawn: X in hand, the N in a column.
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
  // A list of nothing is only its own padding: under the sentence saying the
  // read was incomplete it stood as a blank band the height of two gutters.
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
