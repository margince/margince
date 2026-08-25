// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { RefreshCw } from "lucide-react";
import { navigate } from "../app/router";
import { Button, EmptyState } from "../design-system/atoms";
import {
  BriefItemCard,
  type BriefItemLabels,
} from "../design-system/briefitem";
import { Panel, PanelBody } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatMoneyOrAbsent } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf } from "./common";
import {
  type Deal,
  type MorningBrief,
  useBriefItemMark,
  useBriefRefresh,
} from "./home.queries";

// The ranked half of Home: what the run thinks the day is for, and the three
// verbs that move an entry off it.

/** One brief card's vocabulary. */
function briefLabels(
  t: (key: MessageKey, params?: Record<string, string | number>) => string,
  evidenceCount: number,
): BriefItemLabels {
  return {
    rank: t("home.brief.rank"),
    composite: t("home.brief.composite"),
    factors: {
      winnability: t("home.factorWinnability"),
      revenue: t("home.factorRevenue"),
      timing: t("home.factorTiming"),
      momentum: t("home.factorMomentum"),
      warmth: t("home.factorWarmth"),
    },
    evidence:
      evidenceCount === 1
        ? t("home.evidenceOne")
        : t("home.evidence", { count: evidenceCount }),
    evidenceNone: t("home.evidenceNone"),
    openDeal: t("home.openDeal"),
    act: t("home.act"),
    dismiss: t("home.dismiss"),
    snooze: t("home.snooze"),
    acted: t("home.actedState"),
    dismissed: t("home.dismissedState"),
    snoozed: t("home.snoozedState"),
    resurfaces: t("home.brief.resurfaces"),
  };
}

/** The ranked queue: what the run thinks the day is for. */
export function TodaySection({
  brief,
  deals,
  nowMs,
  state,
}: Readonly<{
  brief: MorningBrief | null;
  deals: readonly Deal[];
  nowMs: number;
  /** What the read behind the queue says. Before it has answered the panel
   *  holds its shape, and a FAILED read says so — `ready` as a boolean could
   *  only tell the two apart by drawing a failure as an empty queue. */
  state: SectionState;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const refresh = useBriefRefresh();
  const mark = useBriefItemMark();

  return (
    <section id="home-today" aria-label={t("home.panel.today")}>
      <Panel
        title={t("home.panel.today")}
        sub={
          brief
            ? t("home.asOf", {
                at: formatDateTime(brief.as_of, locale, viewerZone()),
              })
            : undefined
        }
        titleAction={
          <Button
            small
            disabled={refresh.isPending}
            onClick={() => refresh.mutate()}
            data-testid="brief-refresh"
          >
            <RefreshCw aria-hidden />{" "}
            {refresh.isPending
              ? t("home.refreshing")
              : t(brief ? "home.refresh" : "home.generate")}
          </Button>
        }
        footer={
          brief && brief.items.length > 0 ? (
            <span className="home-honesty t-caption">
              {honestCountLine(t, brief)}
            </span>
          ) : undefined
        }
      >
        <PanelBody className="home-today-list">
          <TodayBody
            brief={brief}
            deals={deals}
            nowMs={nowMs}
            state={state}
            mark={mark}
          />
        </PanelBody>
        {refresh.isError && (
          <PanelBody>
            <p className="home-error t-caption">
              {problemMessageOf(refresh.error, t)}
            </p>
          </PanelBody>
        )}
      </Panel>
    </section>
  );
}

/** The queue's four readings: in flight, no run, a quiet run, or the items. */
function TodayBody({
  brief,
  deals,
  nowMs,
  state,
  mark,
}: Readonly<{
  brief: MorningBrief | null;
  deals: readonly Deal[];
  nowMs: number;
  state: SectionState;
  mark: ReturnType<typeof useBriefItemMark>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Anything but a served read is the state vocabulary's to draw. A failure used
  // to fall through the `ready` guard and render nothing, so a queue nobody
  // could read looked exactly like a morning with nothing in it.
  if (state !== "ready") {
    return (
      <SurfaceState
        state={state}
        emptyLabel={t("home.noneBody")}
        loadingLabel={t("home.panel.today")}
      >
        {null}
      </SurfaceState>
    );
  }
  if (brief === null) {
    return <EmptyState>{t("home.noneBody")}</EmptyState>;
  }
  if (brief.items.length === 0) {
    return <EmptyState>{t("home.quietRun")}</EmptyState>;
  }
  return (
    <>
      {brief.items.map((item) => (
        <BriefItemCard
          key={item.id}
          item={item}
          labels={briefLabels(t, item.evidence_ids.length)}
          // `revenueBasisNote` is deliberately not passed. The run carries
          // `revenue_norm_minor` but not the currency it is in, and the base
          // currency lives on a settings read this page does not make — a sixth
          // request for a footnote. A bare figure with no currency is not a
          // reading, so the card says nothing rather than something
          // unverifiable.
          dealName={deals.find((deal) => deal.id === item.deal_id)?.name}
          amount={amountOf(item.deal_id, deals, locale)}
          formatPercent={(fraction) =>
            t("home.pct", { pct: Math.round(fraction * 100) })
          }
          formatInstant={(utcIso) =>
            formatDateTime(utcIso, locale, viewerZone())
          }
          pending={
            mark.isPending && mark.variables?.itemId === item.id
              ? mark.variables.mark
              : undefined
          }
          error={
            mark.isError && mark.variables?.itemId === item.id
              ? problemMessageOf(mark.error, t)
              : undefined
          }
          onOpenDeal={(dealId) => navigate({ screen: "deals", id: dealId })}
          onAct={(itemId) => mark.mutate({ itemId, mark: "act" })}
          onDismiss={(itemId) => mark.mutate({ itemId, mark: "dismiss" })}
          onSnooze={(itemId) =>
            mark.mutate({
              itemId,
              mark: "snooze",
              snoozedUntil: tomorrowMorning(nowMs),
            })
          }
        />
      ))}
    </>
  );
}

/** What the ranked run left out, said out loud. */
function honestCountLine(
  t: (key: MessageKey, params?: Record<string, string | number>) => string,
  brief: MorningBrief,
): string {
  if (brief.candidate_count > brief.items.length) {
    return t("home.overflow", {
      shown: brief.items.length,
      count: brief.candidate_count,
    });
  }
  return t("home.honestShort", { count: brief.candidate_count });
}

/** One deal's money, or nothing where the page has not read the deal. */
function amountOf(
  dealId: string,
  deals: readonly Deal[],
  locale: Parameters<typeof formatMoneyOrAbsent>[2],
): string | null {
  const deal = deals.find((candidate) => candidate.id === dealId);
  if (!deal) {
    return null;
  }
  return formatMoneyOrAbsent(deal.amount_minor, deal.currency, locale);
}

/**
 * When a snoozed item comes back: tomorrow morning in the reader's own zone.
 *
 * The contract requires a future instant and the product has no snooze picker
 * yet, so this is the policy written once, where a reader can be told it — "back
 * tomorrow" is a promise a morning surface can keep, and the card renders the
 * instant it was given rather than inventing its own.
 */
function tomorrowMorning(nowMs: number): string {
  const back = new Date(nowMs);
  back.setDate(back.getDate() + 1);
  back.setHours(8, 0, 0, 0);
  return back.toISOString();
}
