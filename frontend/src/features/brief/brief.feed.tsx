// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, ChevronRight } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { AppLink } from "../../design-system/applink";
import { Badge, Button, Card, PendingBody } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { PeekCard } from "../../design-system/peek";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useMe } from "../../lib/session";
import { partialFoot } from "../../lib/standingpeeks";
import { useViewerZone } from "../../lib/viewerzone";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import {
  type Worklist,
  type WorklistItem,
  worklistKey,
} from "../worklist/worklist.queries";
import { Triage } from "./brief.focus";
import { identity } from "./brief.focuslink";
import { HomeListReturnScope } from "./brief.queue";
import { laneUnread } from "./brief.readings.honesty";
import { waitingRows } from "./brief.sentence";
import { taskLine, taskName } from "./brief.taskcard";
import { HOME_DAY_TIMING } from "./elementtiming";

import "../worklist/worklist.css";
import "./brief.feed.css";

export function BriefFeed({
  day,
  state,
  changed,
  refreshFailed = false,
  onRetry,
  onContext,
  queueToggle,
}: Readonly<{
  day: Worklist | undefined;
  state: SectionState;
  changed?: Readonly<{ count: number; href: string }>;
  refreshFailed?: boolean;
  onRetry?: () => void;
  onContext?: (item: WorklistItem) => void;
  /** The way into the whole queue, under the day's order. */
  queueToggle?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const titleId = useId();
  const rows = waitingRows(day);
  const partial = Boolean(
    !day?.focus || day.readings?.more_available || laneUnread(day),
  );
  return (
    <section
      id="brief-today"
      className="brief-first"
      aria-labelledby={titleId}
      elementtiming={HOME_DAY_TIMING}
    >
      <div className="brief-first-head">
        <Eyebrow as="h2" id={titleId}>
          {t("brief.first.title")}
        </Eyebrow>
        {changed && (
          <AppLink className="entity-link" href={changed.href}>
            <Badge>
              {plural("brief.feed.changedBadge", changed.count, {
                count: formatNumber(changed.count, locale),
              })}
            </Badge>
          </AppLink>
        )}
        {rows.length > 0 && (
          <span className="t-caption t-num brief-first-count">
            {formatNumber(rows.length, locale)}
          </span>
        )}
        {queueToggle && <div className="brief-first-queue">{queueToggle}</div>}
      </div>
      {refreshFailed && (
        <p role="alert">
          {t("brief.feed.refreshFailed")}{" "}
          <Button variant="ghost" onClick={onRetry}>
            {t("brief.coverage.retry")}
          </Button>
        </p>
      )}
      {/* The card in hand is taller than a few lines of text, so its wait is
          drawn in the card's own frame, and the page does not jump when the
          day lands under it. */}
      {state === "loading" ? (
        <Card as="div" className="brief-triage-lead brief-triage-pending">
          <PendingBody label={t("brief.feed.loading")} lines={6} />
        </Card>
      ) : (
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
          detail={{ onRetry }}
        >
          {rows.length === 0 && partial && <p>{t("brief.feed.incomplete")}</p>}
          <ReadyStack rows={rows} onContext={onContext} />
        </SurfaceState>
      )}
    </section>
  );
}

/** How many rows a narrow panel names under the next one before it counts the rest. */
const POD_ROWS = 2;

/**
 * The day's rows as cards, the one in hand open, with Next. Home's "Needs you"
 * and the agent panel both draw this, so a row reads and answers the same in
 * either place.
 */
export function ReadyStack({
  rows,
  onContext,
  solo = false,
}: Readonly<{
  rows: readonly WorklistItem[];
  onContext?: (item: WorklistItem) => void;
  /** One card at a time with its place in the stack, for a narrow panel. */
  solo?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const client = useQueryClient();
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const { at, choose } = useCardChoice(rows);
  const lead = rows[at];
  const viewer = useMe(false).data?.user;
  const plural = usePlural();
  // A narrow panel lists the day by name first; a press takes one in hand.
  const [inHand, setInHand] = useState(false);
  const listed = solo && !inHand;
  const [first, ...queued] = rows;
  const shown = queued.slice(0, POD_ROWS);
  const next = rows[(at + 1) % rows.length];
  const take = (item: WorklistItem) => {
    choose(item);
    setInHand(true);
  };
  const firstLine = first && taskLine(first, t, locale, readerZone);
  return (
    <>
      {listed && first && (
        <Card as="article" className="brief-pod-hero">
          <div className="brief-pod-hero-text">
            <p className="t-eyebrow">{t("brief.pod.next")}</p>
            <p className="brief-pod-hero-name">
              {taskName(first, viewer, t, locale)}
            </p>
            {firstLine && <p className="t-caption">{firstLine}</p>}
          </div>
          <Button variant="primary" onClick={() => take(first)}>
            {t("brief.pod.open")}
          </Button>
        </Card>
      )}
      {listed && shown.length > 0 && (
        <PeekCard
          head={t("brief.pod.head")}
          headHidden
          items={shown.map((item) => ({
            key: identity(item),
            title: taskName(item, viewer, t, locale),
            right: <ChevronRight aria-hidden className="brief-pod-open" />,
            onOpen: () => take(item),
          }))}
          foot={partialFoot(shown.length + 1, rows.length, {
            t,
            plural,
            locale,
            zone: readerZone,
          })}
        />
      )}
      {!listed && lead && (
        <HomeListReturnScope>
          <Triage
            solo={solo}
            rows={rows}
            at={at}
            lead={lead}
            onChoose={choose}
            onOpenEmail={setOpenEmail}
            onContext={onContext}
          />
        </HomeListReturnScope>
      )}
      {solo && inHand && lead && (
        <div className="brief-pod-pager">
          <Button variant="link" onClick={() => setInHand(false)}>
            <ArrowLeft aria-hidden /> {t("brief.pod.all")}
          </Button>
          {rows.length > 1 && next && (
            <Button variant="link" onClick={() => choose(next)}>
              {t("brief.focus.next")} <ArrowRight aria-hidden />
            </Button>
          )}
        </div>
      )}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={readerZone}
        onClose={() => setOpenEmail(null)}
        onReplySent={() =>
          void client.invalidateQueries({ queryKey: worklistKey })
        }
      />
    </>
  );
}

// The row in hand, by identity. The queue is re-read and answered rows leave
// it, so an index would hand over the row that moved up. A chosen row that has
// gone falls back to the day's first.
function useCardChoice(rows: readonly WorklistItem[]) {
  const [choice, setChoice] = useState<{ key: string | null; at: number }>({
    key: null,
    at: 0,
  });
  const found = rows.findIndex((item) => identity(item) === choice.key);
  const at =
    found >= 0 ? found : Math.max(0, Math.min(choice.at, rows.length - 1));
  if (at !== choice.at) setChoice({ ...choice, at });
  return {
    at,
    choose: (item: WorklistItem) =>
      setChoice({ key: identity(item), at: Math.max(0, rows.indexOf(item)) }),
  };
}
