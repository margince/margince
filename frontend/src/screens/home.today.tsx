// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { RefreshCw } from "lucide-react";
import { Button, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { ProvenanceTag } from "../design-system/trust";
import { formatDateTime, formatMoney, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BriefQueueItem } from "./briefqueue";
import { problemMessageOf } from "./common";
import {
  type Deal,
  type MorningBrief,
  useBriefItemMark,
  useBriefRefresh,
} from "./home.queries";

// The ranked half of Home: what the run thinks the day is for, and the three
// verbs that move an entry off it.

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
            pending={refresh.isPending}
            busyLabel={t("home.refreshing")}
            onClick={() => refresh.mutate()}
            data-testid="brief-refresh"
          >
            <RefreshCw aria-hidden />{" "}
            {t(brief ? "home.refresh" : "home.generate")}
          </Button>
        }
        footer={
          brief && brief.items.length > 0 ? (
            <span className="home-honesty t-caption">
              {honestCountLine(t, brief, locale)}
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

/**
 * What the revenue factor measured against, as money.
 *
 * `undefined` unless the run names BOTH the figure and its currency. A bare
 * number is not money — it reads as whatever currency the reader assumes — and
 * the note exists so a proportion can be checked, which an unnamed base cannot
 * do. A run assembled before the currency was carried names none.
 */
function revenueBasisOf(
  brief: MorningBrief,
  locale: Locale,
): string | undefined {
  if (
    brief.revenue_norm_minor === undefined ||
    brief.revenue_norm_currency === undefined
  ) {
    return undefined;
  }
  return formatMoney(
    brief.revenue_norm_minor,
    brief.revenue_norm_currency,
    locale,
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
    // The narrative still belongs here. A run that ranked nothing is exactly
    // when the night's sentence carries the most — "nothing needs you today"
    // and "nobody looked" are different mornings, and the empty state alone
    // cannot tell them apart.
    return (
      <>
        <TodayNarrative brief={brief} />
        <EmptyState>{t("home.quietRun")}</EmptyState>
      </>
    );
  }
  return (
    <>
      <TodayNarrative brief={brief} />
      {brief.items.map((item) => (
        <BriefQueueItem
          key={item.id}
          item={item}
          deals={deals}
          nowMs={nowMs}
          mark={mark}
          revenueBasis={revenueBasisOf(brief, locale)}
        />
      ))}
    </>
  );
}

/**
 * The sentence about the night, above the queue.
 *
 * THREE STATES, not two, and the third is the one worth the code. A run with no
 * narrative can mean the overnight pass ran and honestly had nothing to say, or
 * that it never ran at all — no grant, no model, an exhausted budget. Those look
 * identical on the row and read identically as silence, so `annotated_at` is
 * what separates them and the screen says which.
 *
 * Saying nothing in the third case would be the dishonest option: the rep would
 * read a queue with no explanation and conclude the product had nothing to
 * explain, when in fact nobody looked.
 */
function TodayNarrative({ brief }: Readonly<{ brief: MorningBrief }>) {
  const t = useT();
  if (!brief.annotated_at) {
    return (
      <p className="home-narrative home-narrative-absent t-caption">
        {t("home.narrativeNoPass")}
      </p>
    );
  }
  if (!brief.narrative) {
    return null;
  }
  return (
    <div className="home-narrative">
      <ProvenanceTag provenance={{ kind: "agent" }} />
      <p>{brief.narrative}</p>
    </div>
  );
}

/** What the ranked run left out, said out loud. */
function honestCountLine(
  t: (key: MessageKey, params?: Record<string, string>) => string,
  brief: MorningBrief,
  locale: Parameters<typeof formatNumber>[1],
): string {
  if (brief.candidate_count > brief.items.length) {
    return t("home.overflow", {
      shown: formatNumber(brief.items.length, locale),
      count: formatNumber(brief.candidate_count, locale),
    });
  }
  return t("home.honestShort", {
    count: formatNumber(brief.candidate_count, locale),
  });
}
