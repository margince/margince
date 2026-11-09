// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { AppLink } from "../../design-system/applink";
import { Badge, Button, Card, PendingBody } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import {
  type Worklist,
  type WorklistItem,
  worklistKey,
} from "../worklist/worklist.queries";
import { CardPager } from "./brief.cardpager";
import { Triage } from "./brief.focus";
import { identity } from "./brief.focuslink";
import { HomeListReturnScope } from "./brief.queue";
import { laneUnread } from "./brief.readings.honesty";
import { waitingRows } from "./brief.sentence";
import { HOME_DAY_TIMING } from "./elementtiming";

import "../worklist/worklist.css";
import { useViewerZone } from "../../lib/viewerzone";
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
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  // The row in hand, by identity. The queue is re-read and answered rows
  // leave it, so an index would hand over the row that moved up. A chosen
  // row that has gone falls back to the day's first.
  const client = useQueryClient();
  const titleId = useId();
  const rows = waitingRows(day);
  const { at, choose } = useCardChoice(rows);
  const lead = rows[at];
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
          {lead && (
            <HomeListReturnScope>
              <FeedPager
                rows={rows}
                at={at}
                choose={choose}
                title={lead.title ?? ""}
              >
                <Triage
                  rows={rows}
                  at={at}
                  lead={lead}
                  onChoose={choose}
                  onOpenEmail={setOpenEmail}
                  onContext={onContext}
                />
              </FeedPager>
            </HomeListReturnScope>
          )}
        </SurfaceState>
      )}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={readerZone}
        onClose={() => setOpenEmail(null)}
        onReplySent={() =>
          void client.invalidateQueries({ queryKey: worklistKey })
        }
      />
    </section>
  );
}

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

function FeedPager({
  rows,
  at,
  choose,
  title,
  children,
}: Readonly<{
  rows: readonly WorklistItem[];
  at: number;
  choose: (item: WorklistItem) => void;
  title: string;
  children: ReactNode;
}>) {
  const previous = rows[at - 1];
  const next = rows[at + 1];
  return (
    <CardPager
      previous={previous ? () => choose(previous) : undefined}
      next={next ? () => choose(next) : undefined}
      title={title}
    >
      {children}
    </CardPager>
  );
}
