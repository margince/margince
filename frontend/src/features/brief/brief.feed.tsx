// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { AppLink } from "../../design-system/applink";
import { Badge, Button } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import {
  type Worklist,
  type WorklistItem,
  worklistKey,
} from "../worklist/worklist.queries";
import { identity, Triage } from "./brief.focus";
import { HomeListReturnScope } from "./brief.queue";
import { laneUnread } from "./brief.readings.honesty";
import { waitingRows } from "./brief.sentence";

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
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  // The row in hand, by identity. The queue is re-read and answered rows
  // leave it, so an index would hand over the row that moved up. A chosen
  // row that has gone falls back to the day's first.
  const [chosen, setChosen] = useState<string | null>(null);
  const client = useQueryClient();
  const titleId = useId();
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
    <section id="brief-today" className="brief-first" aria-labelledby={titleId}>
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
        {lead && (
          <span className="t-caption t-num brief-first-position">
            {t("brief.focus.position", {
              at: formatNumber(at + 1, locale),
              count: formatNumber(rows.length, locale),
            })}
          </span>
        )}
      </div>
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
        {rows.length === 0 && partial && <p>{t("brief.feed.incomplete")}</p>}
        {lead && (
          <HomeListReturnScope>
            <Triage
              rows={rows}
              at={at}
              lead={lead}
              onChoose={(item) => setChosen(identity(item))}
              onOpenEmail={setOpenEmail}
              onContext={onContext}
            />
          </HomeListReturnScope>
        )}
      </SurfaceState>
      {queueToggle && <div className="brief-first-foot">{queueToggle}</div>}
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
