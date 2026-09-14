import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  Skeleton,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { AutonomyDot } from "../design-system/trust";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";

type Activity = components["schemas"]["Activity"];
type TranscriptReadReport = components["schemas"]["TranscriptReadReport"];

// A transcript is a spoken conversation the rep pasted or uploaded, so only a
// call or a meeting can carry one — `source_system` is an idempotency-key part
// on every kind, and a mail connector stamping some future value there must not
// put a "read this transcript" offer on an email.
export function isTranscriptActivity(activity: Activity): boolean {
  return (
    (activity.kind === "call" || activity.kind === "meeting") &&
    activity.source_system === "transcript"
  );
}

const READ_STATUS_LABELS: Record<TranscriptReadReport["status"], MessageKey> = {
  queued: "transcriptread.statusQueued",
  running: "transcriptread.statusRunning",
  done: "transcriptread.statusDone",
  failed: "transcriptread.statusFailed",
};

// The outcome of a finished reading, as the three answers a rep must be able to
// tell apart. "It stated nothing" and "it could not be read" are different
// facts about the transcript, and one drawn as the other either buries a
// correct empty answer or dresses a failure as one.
function TranscriptReadOutcome({
  report,
}: Readonly<{ report: TranscriptReadReport }>) {
  const t = useT();
  if (report.status === "failed") {
    return (
      <Callout
        tone="danger"
        kind="event"
        title={t("transcriptread.failedTitle")}
      >
        {report.status_detail ?? t("transcriptread.failedFallback")}
      </Callout>
    );
  }
  if (report.proposal_ids.length === 0) {
    // A reading that stated nothing is the EMPTY arm of the same switch whose
    // other arm lists the proposals — a fact about the transcript, not
    // something the card says about itself.
    return (
      <EmptyState>
        {report.status_detail ?? t("transcriptread.nothingStated")}
      </EmptyState>
    );
  }
  return <TranscriptReadProposals ids={report.proposal_ids} />;
}

// What became of what the reading staged.
//
// `proposal_ids` is a FROZEN record of what this run proposed — it is written
// once and never touched again — and the card read its length under the words
// "waiting for your review". So a rep who accepted the only suggestion, and
// watched the task appear, came back to a card still saying one was waiting.
// Nothing was wrong with the number; the sentence was about a different fact.
//
// The decisions are read where they actually live, one authorised read per
// staged approval, keyed ["approval", id] — the key approvalrow already
// invalidates when a decision lands, so accepting from the worklist refreshes
// this card without a reload.
//
// A hook cannot sit under a changing early return, which is why this is its own
// component: the parent returns before it for a failed or empty reading.
function TranscriptReadProposals({ ids }: Readonly<{ ids: string[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const decisions = useQueries({
    queries: ids.map((id) => ({
      queryKey: ["approval", id],
      queryFn: async () => {
        const { data, error } = await api.GET("/approvals/{id}", {
          params: { path: { id } },
        });
        if (error) {
          throwProblem(error);
        }
        return data;
      },
      // A decided approval does not un-decide, and this card is often on
      // screen while the rep works elsewhere in the record.
      staleTime: 10_000,
      retry: false,
    })),
  });

  const settled = decisions.filter((one) => !one.isPending);
  const known = settled.filter((one) => !one.isError).map((one) => one.data);
  const unreadable = settled.length - known.length;
  const pending = known.filter((one) => one?.status === "pending").length;
  const approved = known.filter((one) => one?.status === "approved").length;
  const rejected = known.filter((one) => one?.status === "rejected").length;
  const expired = known.filter((one) => one?.status === "expired").length;
  // An approved suggestion whose effect did not run has NOT produced the task
  // it promised. Counting it as done would tell a rep the work exists when it
  // does not — the one reading of this card that costs them a commitment.
  const effectFailed = known.filter((one) => one?.effect_failed_at).length;
  // Only what a CONTACT actually decided. An approval that lapsed was reviewed
  // by nobody, and one whose status could not be read is unknown rather than
  // settled — counting either as reviewed makes the card claim an answer that
  // was never given, next to a line saying the opposite.
  const reviewed = approved + rejected;

  // Until every read has answered, the historical count is all that can be
  // said honestly. Saying "reviewed" early would claim decisions not yet seen,
  // and saying "waiting" early is the bug this replaced.
  const loading = settled.length < ids.length;

  return (
    <>
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
          {loading
            ? plural("transcriptread.staged", ids.length, {
                count: formatNumber(ids.length, locale),
              })
            : pending > 0
              ? plural("transcriptread.proposals", pending, {
                  count: formatNumber(pending, locale),
                })
              : plural("transcriptread.decided", reviewed, {
                  count: formatNumber(reviewed, locale),
                })}
        </span>
        {!loading && pending === 0 && (approved > 0 || rejected > 0) && (
          <span className="t-caption">
            {t("transcriptread.decidedDetail", {
              accepted: formatNumber(approved, locale),
              rejected: formatNumber(rejected, locale),
            })}
          </span>
        )}
        {!loading && expired > 0 && (
          <span className="t-caption">
            {plural("transcriptread.expired", expired, {
              count: formatNumber(expired, locale),
            })}
          </span>
        )}
        {unreadable > 0 && (
          <span className="t-caption">
            {plural("transcriptread.statusUnknown", unreadable, {
              count: formatNumber(unreadable, locale),
            })}
          </span>
        )}
        {/* The worklist is where a pending suggestion is decided. Once none is,
            it is still where the decided ones are listed, so the button keeps
            leading somewhere real rather than disappearing. */}
        <Button small onClick={() => navigate({ screen: "worklist" })}>
          {t("enrich.toInbox")}
        </Button>
      </p>
      {/* AFTER the line, never inside it: a notice is a div, and the line above
          is a paragraph of spans. */}
      {effectFailed > 0 && (
        <Callout
          tone="danger"
          kind="event"
          title={t("transcriptread.effectFailedTitle")}
        >
          {plural("transcriptread.effectFailed", effectFailed, {
            count: formatNumber(effectFailed, locale),
          })}
        </Callout>
      )}
    </>
  );
}

// The polled half of the reading: a live status badge while the model works
// (3s poll, stopping the moment the status is terminal), then how much was read
// and what came of it.
function TranscriptReadPanel({
  activityId,
  readId,
}: Readonly<{ activityId: string; readId: string }>) {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  const reportQuery = useQuery({
    queryKey: ["transcript-read", activityId, readId],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/activities/{id}/transcript-proposals/{readId}",
        { params: { path: { id: activityId, readId } } },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "queued" || status === "running" ? 3000 : false;
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
  const terminal = report.status === "done" || report.status === "failed";

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
          {t(READ_STATUS_LABELS[report.status])}
        </Badge>
        {terminal && (
          <span className="t-caption">
            {plural("transcriptread.lineCount", report.line_count, {
              count: formatNumber(report.line_count, locale),
            })}
          </span>
        )}
      </p>
      {terminal && <TranscriptReadOutcome report={report} />}
    </div>
  );
}

/**
 * TranscriptReadCard offers one reading of one transcript (S-E04.3): the model
 * reads the normalized lines and stages every next step the conversation
 * STATES as a 🟡 proposal, citing the lines it read them from. Nothing reaches
 * the timeline until a human confirms in the inbox.
 *
 * 422 (this activity carries no transcript) and 501 (no model configured)
 * surface their honest cause rather than a generic failure.
 */
export function TranscriptReadCard({
  activityId,
}: Readonly<{ activityId: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [readId, setReadId] = useState<string | null>(null);
  // A read id lives only in the tab that started the reading, so one that
  // finished after the rep navigated away would be unfindable — and a
  // transcript nobody had read would look exactly like one whose reading
  // failed. 404 is the honest "never read" and leaves the card offering a
  // first reading.
  const latest = useQuery({
    queryKey: ["transcript-read-latest", activityId],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/activities/{id}/transcript-proposals/latest",
        { params: { path: { id: activityId } } },
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
  const shownReadId = readId ?? latest.data?.read_id ?? null;
  const start = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await api.POST(
        "/activities/{id}/transcript-proposals",
        { params: { path: { id: activityId } } },
      );
      if (error) {
        // 501 means no model is wired on this server, which the card states in
        // the reader's terms. Either way this stays a problem, so the render
        // below can tell it from a bug in here.
        throwProblem(
          response.status === 501
            ? { title: t("transcriptread.unavailable") }
            : error,
        );
      }
      return data;
    },
    onSuccess: (started) => {
      setReadId(started.read_id);
      // The started reading IS the latest one, so say so rather than leaving
      // the cached answer to expire: without this the card holds a stale
      // "never read" that a rep who navigates away and back inside the window
      // still sees.
      queryClient.invalidateQueries({
        queryKey: ["transcript-read-latest", activityId],
      });
    },
  });

  return (
    <Card
      title={t("transcriptread.title")}
      sub={t("transcriptread.sub")}
      actions={
        <Button
          small
          pending={start.isPending}
          busyLabel={t("transcriptread.starting")}
          onClick={() => start.mutate()}
        >
          {t("transcriptread.cta")}
        </Button>
      }
      style={{ marginTop: "var(--space-3)" }}
    >
      {latest.isPending && <Skeleton width="40%" />}
      {/* A 404 is "never read" and resolves to null; anything else means we do
          not KNOW whether this transcript has been read. Saying so beats an
          empty card, which reads as a confident "not yet". */}
      {latest.isError && (
        <p className="t-caption" style={{ color: "var(--dangerText)" }}>
          {problemMessageOf(latest.error, t)}
        </p>
      )}
      {start.isError && (
        <p className="t-caption" style={{ color: "var(--dangerText)" }}>
          {problemMessageOf(start.error, t)}
        </p>
      )}
      {shownReadId && (
        <TranscriptReadPanel activityId={activityId} readId={shownReadId} />
      )}
    </Card>
  );
}
