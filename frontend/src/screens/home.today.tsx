// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { RefreshCw } from "lucide-react";
import { Button, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
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
        <BriefQueueItem
          key={item.id}
          item={item}
          deals={deals}
          nowMs={nowMs}
          mark={mark}
        />
      ))}
    </>
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
