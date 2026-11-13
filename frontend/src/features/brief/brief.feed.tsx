// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId } from "react";
import { AppLink } from "../../design-system/applink";
import { Badge, Button, Card, PendingBody } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { Worklist, WorklistItem } from "../worklist/worklist.queries";
import { laneUnread } from "./brief.readings.honesty";
import { waitingRows } from "./brief.sentence";
import { ReadyStack } from "./brief.stack";
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
