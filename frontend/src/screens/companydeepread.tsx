// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The company's website read, end to end: the panel that offers it, the panel
// that reports on it, and the vocabulary both use. It lives apart from the
// company screens because it is one self-contained conversation with a rep —
// shall we read this site, and what did we find — rather than another pane of
// the record.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { watchStartedAiRun } from "../app/ai-activity";
import { navigate } from "../app/router";
import { Badge, Button, Skeleton } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { AutonomyDot } from "../design-system/trust";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import { type ConfiguredStopReason, stopIsConfigured } from "./sitereadkind";

type SiteReadReport = components["schemas"]["SiteReadReport"];

const SITE_READ_STATUS_LABELS: Record<SiteReadReport["status"], MessageKey> = {
  queued: "deepread.statusQueued",
  deferred: "deepread.statusDeferred",
  running: "deepread.statusRunning",
  done: "deepread.statusDone",
  partial: "deepread.statusPartial",
  cancelled: "deepread.statusCancelled",
  failed: "deepread.statusFailed",
};

// How far a read has got, as the two stages the engine actually reports.
//
// TWO, not the four a reader might imagine — the server's `phase` says
// `crawling` or `extracting` and nothing finer, and a ladder with invented
// rungs would be a progress bar that moves on its own schedule. Two honest
// stages tell a reader more than four made-up ones.
export type ScanState = "done" | "running" | "queued";

const SCAN_STAGES = ["crawling", "extracting"] as const;

type ScanStage = (typeof SCAN_STAGES)[number];

const SCAN_STAGE_LABELS: Record<ScanStage, MessageKey> = {
  crawling: "deepread.stage.crawling",
  extracting: "deepread.stage.extracting",
};

/**
 * Where each stage stands, read off the report rather than timed.
 *
 * A read that has not started has both stages waiting; one in `extracting` has
 * finished crawling by definition, which is the only inference here and it is
 * the engine's own ordering rather than a guess about elapsed time.
 */
export function scanStates(
  report: SiteReadReport,
): Record<ScanStage, ScanState> {
  if (report.status === "queued" || report.status === "deferred") {
    return { crawling: "queued", extracting: "queued" };
  }
  // `phase` is null the moment a read goes terminal, so a running read with no
  // phase is one whose first stage is under way and has not said so yet.
  return report.phase === "extracting"
    ? { crawling: "done", extracting: "running" }
    : { crawling: "running", extracting: "queued" };
}

/**
 * The stages of a read in flight, as a ladder.
 *
 * Drawn ONLY while the read is still going. Once it is done, stopped or
 * failed, the panel beside this says so with its own reason, and a ladder of
 * ticks under a failure would be the page congratulating itself.
 */
function ScanSteps({ report }: Readonly<{ report: SiteReadReport }>) {
  const t = useT();
  // Every state the ladder can answer for, and only those. A read waiting on
  // budget is still a read in flight — the deferral note beside this says how
  // long — while a finished or failed one has its own reason to give, and a
  // column of ticks under a failure would be the page congratulating itself.
  const inFlight =
    report.status === "queued" ||
    report.status === "deferred" ||
    report.status === "running";
  if (!inFlight) {
    return null;
  }
  const states = scanStates(report);
  return (
    <ol className="deepread-steps">
      {SCAN_STAGES.map((stage) => (
        <li key={stage} className={`deepread-step t-sub is-${states[stage]}`}>
          <span className="deepread-step-mark" aria-hidden="true" />
          {t(SCAN_STAGE_LABELS[stage])}
          {/* The state in words as well as in the mark: three tinted circles
              are three tinted circles to a reader who cannot tell them
              apart. */}
          <span className="deepread-step-state t-caption">
            {t(`deepread.step.${states[stage]}`)}
          </span>
        </li>
      ))}
    </ol>
  );
}

type SiteReadStopReason = NonNullable<SiteReadReport["stopped_reason"]>;

/**
 * What a read that spent one of its own ceilings is CALLED.
 *
 * Exhaustive over `ConfiguredStopReason`, so adding a ceiling to that list in
 * sitereadkind.ts fails the build here until this names it. One line per
 * ceiling: a crawl stopped by the byte cap did not reach a page limit, and the
 * point of naming a configured stop is that the name is right about which
 * budget ran out.
 */
const SITE_READ_CAPPED_LABELS: Record<ConfiguredStopReason, MessageKey> = {
  page_cap: "deepread.statusPageCapped",
  byte_cap: "deepread.statusByteCapped",
  deadline: "deepread.statusTimeCapped",
};

/**
 * Why a read was INTERRUPTED, for the warn badge.
 *
 * Only the stops a later run might get past appear here. A configured ceiling
 * is named by the status instead, so a label for one would be unreachable copy
 * — the kind a translator keeps current and nobody ever sees.
 */
const SITE_READ_STOP_LABELS: Record<
  Exclude<SiteReadStopReason, ConfiguredStopReason>,
  MessageKey
> = {
  budget: "deepread.stopBudget",
};

/**
 * What to call a read's outcome.
 *
 * The wire says `partial` for every bounded read, which lumps two different
 * events under one word: a crawl that spent its configured page or byte budget,
 * and one that lost its model budget or its wall clock. The first is a complete
 * run of the size an operator asked for, so it is named for what it did rather
 * than for what it stopped short of.
 */
function statusLabelOf(report: SiteReadReport): MessageKey {
  if (report.status !== "partial" || !report.stopped_reason) {
    return SITE_READ_STATUS_LABELS[report.status];
  }
  return stopIsConfigured(report.stopped_reason)
    ? SITE_READ_CAPPED_LABELS[report.stopped_reason]
    : SITE_READ_STATUS_LABELS[report.status];
}

function SiteReadDeferral({ report }: Readonly<{ report: SiteReadReport }>) {
  const t = useT();
  const { locale } = useLocale();
  if (report.status !== "deferred") {
    return null;
  }
  return (
    <p className="t-caption" style={{ margin: "var(--space-2) 0 0" }}>
      {report.status_detail}
      {report.next_attempt_at && (
        <>
          {" "}
          {t("deepread.resumesAt", {
            when: formatDateTime(report.next_attempt_at, locale, viewerZone()),
          })}
        </>
      )}
    </p>
  );
}

// The polled half of the deep read: renders progress while the crawl is in
// flight (3s poll, stops on a terminal status) and the full account when it
// ends — pages read, pages SKIPPED and why, and the stop reason when the
// crawl ended early. The skip/stop rendering is the transparency surface: a
// truncated crawl must never read as complete.
function SiteReadPanel({
  companyId,
  readId,
}: Readonly<{ companyId: string; readId: string }>) {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  const reportQuery = useQuery({
    queryKey: ["site-read", companyId, readId],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/companies/{id}/site-reads/{readId}",
        { params: { path: { id: companyId, readId } } },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === "queued" || status === "running") {
        return 3000;
      }
      return status === "deferred" ? 60_000 : false;
    },
  });

  if (reportQuery.isPending) {
    return <Skeleton width="60%" />;
  }
  if (reportQuery.isError) {
    return (
      <p className="t-caption" style={{ color: "var(--dangerText)" }}>
        {problemMessageOf(reportQuery.error, t)}
      </p>
    );
  }

  const report = reportQuery.data;
  const terminal =
    report.status === "done" ||
    report.status === "partial" ||
    report.status === "failed";

  return (
    <div style={{ marginTop: "var(--space-3)" }}>
      <p
        style={{
          display: "flex",
          alignItems: "center",
          gap: "var(--space-2)",
          flexWrap: "wrap",
          margin: 0,
        }}
      >
        <Badge tone={report.status === "failed" ? "danger" : undefined}>
          {t(statusLabelOf(report))}
        </Badge>
        <span className="t-caption">
          {plural("deepread.pagesSoFar", report.pages.length, {
            count: formatNumber(report.pages.length, locale),
          })}
        </span>
        {terminal && (
          <span className="t-caption">
            {plural("deepread.factCount", report.fact_count ?? 0, {
              count: formatNumber(report.fact_count ?? 0, locale),
            })}
          </span>
        )}
      </p>
      <ScanSteps report={report} />
      <SiteReadDeferral report={report} />
      {/* Only the stops a reader can do something about are drawn as a
          warning. A page or byte cap is already stated by the status beside
          the page count, and repeating it in a warn badge told a rep their
          read had gone wrong when it had done exactly what it was configured
          to do. */}
      {report.stopped_reason && !stopIsConfigured(report.stopped_reason) && (
        <p style={{ margin: "var(--space-2) 0 0" }}>
          <Badge tone="warn">
            {t("deepread.stoppedEarly", {
              reason: t(SITE_READ_STOP_LABELS[report.stopped_reason]),
            })}
          </Badge>
        </p>
      )}
      {terminal && report.proposal_ids.length > 0 && (
        <p
          style={{
            display: "flex",
            alignItems: "center",
            gap: "var(--space-2)",
            flexWrap: "wrap",
            margin: "var(--space-3) 0 0",
          }}
        >
          <AutonomyDot tier="confirm" />
          <span className="t-caption">
            {plural("deepread.proposals", report.proposal_ids.length, {
              count: formatNumber(report.proposal_ids.length, locale),
            })}
          </span>
          <Button small onClick={() => navigate({ screen: "worklist" })}>
            {t("enrich.toInbox")}
          </Button>
        </p>
      )}
    </div>
  );
}

/**
 * The account's most recent website read, or null when it has never been read.
 *
 * A read id lives only in the tab that started the crawl, so a read that ended
 * after the rep navigated away would be unfindable, and a FAILED crawl would
 * look like one nobody tried. 404 is the honest "never read".
 *
 * Shared by the panel and by the page deciding where to PUT the panel, on one
 * query key so the two are one request and can never disagree about whether
 * this account has been read.
 */
function useLatestSiteRead(companyId: string) {
  return useQuery({
    queryKey: ["site-read-latest", companyId],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/companies/{id}/site-reads/latest",
        { params: { path: { id: companyId } } },
      );
      if (response.status === 404) {
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data ?? null;
    },
  });
}

/**
 * Whether this account has been researched already.
 *
 * FALSE while the answer is unknown — still loading, or the lookup failed.
 * The caller uses this to decide whether the Overview leads with the research
 * offer, and an account wrongly called "read" would hide the offer from the
 * one company that needs it. An offer shown a moment too long costs nothing.
 */
export function useHasSiteRead(companyId: string): boolean {
  return useLatestSiteRead(companyId).data != null;
}

// The whole-site deep read, the enrich verb's big sibling: one click starts
// (or joins — idempotent per company+url) a background crawl of the company's own
// site; findings stage as 🟡 proposals for the inbox, nothing writes to the
// record here. 422 (no website) and 501 (crawl seam unwired) say their cause.
export function DeepReadPanel({ companyId }: Readonly<{ companyId: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [readId, setReadId] = useState<string | null>(null);
  const latest = useLatestSiteRead(companyId);
  const shownReadId = readId ?? latest.data?.read_id ?? null;
  const start = useMutation({
    mutationKey: ["site-read", companyId],
    mutationFn: async () => {
      const { data, error, response } = await api.POST(
        "/companies/{id}/deep-read",
        { params: { path: { id: companyId } } },
      );
      if (error) {
        // 501 means the crawl seam is unwired. Either way this stays a
        // problem, so the render below can tell it from a bug in here.
        throwProblem(
          response.status === 501
            ? { title: t("deepread.unavailable") }
            : error,
        );
      }
      return data;
    },
    onSuccess: (started) => {
      setReadId(started.read_id);
      // The started read IS the latest one, so say so rather than leaving the
      // cached answer to expire: otherwise a rep who navigates away and back
      // inside the 30s window still meets a stale "never read".
      queryClient.invalidateQueries({
        queryKey: ["site-read-latest", companyId],
      });
      // The occurrence reaches the rail's feed through the outbox, so the
      // crawl is not on the Core when the 202 lands. This is what makes the
      // rail watch for it rather than meet it on its next idle poll.
      watchStartedAiRun(queryClient);
    },
  });

  // What the panel is FOR, which changes the moment a read exists. Until then
  // it offers a capability nobody here has used, and the two sentences
  // explaining it are the point. After that the offer has been answered: the
  // reader wants the last read's outcome and a way to run it again, and a pitch
  // for something already done reads as though the read never happened.
  //
  // A KNOWN read is what stands the offer down — not merely the absence of a
  // pitch-worthy answer. While `latest` is still in flight, and if it fails
  // outright, there is no read to report on, so the panel keeps offering: the
  // worst case is a rep reading one sentence too many, where the other way
  // round is a header promising a report the panel cannot draw.
  const offering = shownReadId === null;

  return (
    // The badge and not the tint: this zone OFFERS an AI verb, while its body
    // is the crawl's own status — indigo would claim a machine wrote that.
    <Panel
      title={t(offering ? "deepread.title" : "deepread.titleRead")}
      titleAction={<Badge tone="ai">{t("co.assistant.aiTag")}</Badge>}
      actions={
        <Button
          small
          pending={start.isPending}
          busyLabel={t("deepread.starting")}
          onClick={() => start.mutate()}
        >
          {t(offering ? "deepread.cta" : "deepread.ctaAgain")}
        </Button>
      }
    >
      <PanelBody>
        {/* Two sentences, so the head's one line would truncate the half that
            says nothing is written until a contact accepts it. Drawn only while
            the panel is still an offer — see `offering`. */}
        {offering && <p className="t-sub">{t("deepread.sub")}</p>}
        {start.isError && (
          <p className="t-caption" style={{ color: "var(--dangerText)" }}>
            {problemMessageOf(start.error, t)}
          </p>
        )}
        {shownReadId && (
          <SiteReadPanel companyId={companyId} readId={shownReadId} />
        )}
      </PanelBody>
    </Panel>
  );
}
