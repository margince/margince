import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  showsAMessage,
  TASK_QUEUE_KEY,
  transcriptReadLatestKey,
} from "../lib/activitykeys";
import { labelFor } from "../lib/labelfor";
import { throwProblem, unwrap } from "../lib/problem";
import { navigate } from "../lib/router";
import { worklistKey } from "../lib/workkeys";
import { Badge, Button, Card, EmptyState, Skeleton } from "./atoms";
import { Callout } from "./callout";
import { ErrorLine } from "./errorline";
import { Eyebrow } from "./eyebrow";
import { AutonomyDot, formatSourceLines } from "./trust";
import "./transcriptread.css";
import { approvalQuery } from "../lib/approvalread";

type Activity = components["schemas"]["Activity"];
type TranscriptReadReport = components["schemas"]["TranscriptReadReport"];
type TranscriptSummaryPoint = components["schemas"]["TranscriptSummaryPoint"];

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
  // `detail` is phrased for this reader by the server; `status_detail` is the
  // stored sentence a reading from before carries instead.
  const detail = report.detail ?? report.status_detail;
  if (report.status === "failed") {
    return (
      <Callout
        tone="danger"
        kind="event"
        title={t("transcriptread.failedTitle")}
      >
        {detail ?? t("transcriptread.failedFallback")}
      </Callout>
    );
  }
  // A refused summary leaves the meeting's earlier one in place, and the box
  // says this reading wrote none.
  return (
    <>
      <TranscriptReadFound report={report} detail={detail} />
      {report.summary_refused && (
        <p className="t-caption">{t("transcriptread.summaryRefused")}</p>
      )}
    </>
  );
}

// What a done reading found: the proposals it staged, or the empty answer.
function TranscriptReadFound({
  report,
  detail,
}: Readonly<{ report: TranscriptReadReport; detail?: string | null }>) {
  const t = useT();
  if (report.proposal_ids.length === 0) {
    // A reading that stated nothing is the EMPTY arm of the same switch whose
    // other arm lists the proposals — a fact about the transcript, not
    // something the card says about itself.
    return (
      <EmptyState>{detail ?? t("transcriptread.nothingStated")}</EmptyState>
    );
  }
  const mine = ownProposals(report);
  return (
    <>
      {report.detail && <p className="t-caption">{report.detail}</p>}
      {mine.length > 0 && <TranscriptReadProposals ids={mine} />}
    </>
  );
}

// The proposals this reader decides. One waiting on a colleague is theirs, and
// its status is not this reader's to read: the server's sentence already says
// who it was sent to. A reading from before names nobody, so all of its
// proposals are read as before.
function ownProposals(report: TranscriptReadReport): string[] {
  if (report.proposals.length === 0) {
    return report.proposal_ids;
  }
  return report.proposals
    .filter((proposal) => proposal.asked_of.is_you)
    .map((proposal) => proposal.id);
}

// What became of what the reading staged.
//
// `proposal_ids` is a FROZEN record of what this run proposed — it is written
// once and never touched again — and the card read its length under the words
// "waiting for your review". So a rep who accepted the only suggestion, and
// watched the task appear, came back to a card still saying one was waiting.
// Nothing was wrong with the number; the sentence was about a different fact.
//
// The decisions are read where they actually live: one authorised read per
// staged approval, through `approvalQuery`.
//
// That is the key approvalrow invalidates when a decision lands, so accepting
// from the worklist refreshes this card without a reload.
//
// A hook cannot sit under a changing early return, which is why this is its own
// component: the parent returns before it for a failed or empty reading.
function TranscriptReadProposals({ ids }: Readonly<{ ids: string[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const decisions = useQueries({
    queries: ids.map((id) => ({
      ...approvalQuery(id),
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
      <p className="transcript-tally">
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
        <Button onClick={() => navigate({ screen: "worklist" })}>
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
  const reportQuery = useQuery({
    queryKey: ["transcript-read", activityId, readId],
    queryFn: async () => {
      return unwrap(
        await api.GET("/activities/{id}/transcript-proposals/{readId}", {
          params: { path: { id: activityId, readId } },
        }),
      );
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
    return <ErrorLine error={reportQuery.error} />;
  }

  const report = reportQuery.data;
  const terminal = report.status === "done" || report.status === "failed";
  return (
    <TranscriptReadBody
      report={report}
      terminal={terminal}
      activityId={activityId}
    />
  );
}

// Split from the polling component so the refresh below can sit in an effect
// that runs only once the data exists.
function TranscriptReadBody({
  report,
  terminal,
  activityId,
}: Readonly<{
  report: TranscriptReadReport;
  terminal: boolean;
  activityId: string;
}>) {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const status = report.status;
  const readId = report.read_id;
  const previous = useRef(status);
  useEffect(() => {
    const wasWorking =
      previous.current === "queued" || previous.current === "running";
    previous.current = status;
    if (status !== "done") {
      return;
    }
    if (!wasWorking) {
      // A reading first seen done may have finished after the shared latest
      // read was fetched, and that read carries the summary.
      const latest = queryClient.getQueryData<TranscriptReadReport | null>(
        transcriptReadLatestKey(activityId),
      );
      if (latest?.read_id !== readId || latest.status !== "done") {
        queryClient.invalidateQueries({
          queryKey: transcriptReadLatestKey(activityId),
        });
      }
      return;
    }
    // The reading wrote tasks and findings onto records this card is not told
    // about. So every cached timeline, 360 first page, task and worklist read is
    // refreshed; only mounted queries refetch.
    queryClient.invalidateQueries({ predicate: showsAMessage });
    queryClient.invalidateQueries({ queryKey: TASK_QUEUE_KEY });
    queryClient.invalidateQueries({ queryKey: worklistKey });
    queryClient.invalidateQueries({ queryKey: ["activity", activityId] });
    queryClient.invalidateQueries({
      queryKey: transcriptReadLatestKey(activityId),
    });
  }, [status, readId, queryClient, activityId]);

  return (
    <div className="transcript-reading">
      <p className="transcript-status">
        <Badge tone={report.status === "failed" ? "danger" : undefined}>
          {t(labelFor(READ_STATUS_LABELS, report.status))}
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
 * TranscriptReadCard offers one reading of one transcript (S-E04.3). The model
 * reads the next steps the conversation STATES, each citing its lines. A staged
 * proposal waits for a human to confirm it. The summary is shown at once.
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
  const latest = useLatestTranscriptRead(activityId);
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
        queryKey: transcriptReadLatestKey(activityId),
      });
    },
  });

  return (
    <Card
      title={t("transcriptread.title")}
      actions={
        <Button
          pending={start.isPending}
          busyLabel={t("transcriptread.starting")}
          onClick={() => start.mutate()}
        >
          {t("transcriptread.cta")}
        </Button>
      }
      className="transcript-card"
    >
      {latest.isPending && <Skeleton width="40%" />}
      {/* A 404 is "never read" and resolves to null; anything else means we do
          not KNOW whether this transcript has been read. Saying so beats an
          empty card, which reads as a confident "not yet". */}
      <ErrorLine error={latest.error} />
      <ErrorLine error={start.error} />
      {shownReadId && (
        <TranscriptReadPanel activityId={activityId} readId={shownReadId} />
      )}
    </Card>
  );
}

// The newest reading of one transcript. The summary above the transcript and
// the reading card below it share this one fetch.
function useLatestTranscriptRead(activityId: string) {
  return useQuery({
    queryKey: transcriptReadLatestKey(activityId),
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
}

/**
 * TranscriptSummary is the short summary a reading wrote onto a meeting, drawn
 * above its transcript with the AI mark. Each point names the lines it was read
 * from. It draws nothing until a reading has written one, and a reading whose
 * summary was refused leaves the earlier one in place.
 */
export function TranscriptSummary({
  activityId,
}: Readonly<{ activityId: string }>) {
  const t = useT();
  const latest = useLatestTranscriptRead(activityId);
  // A failed re-read hides the summary rather than keep words an erasure may
  // have removed.
  const summary = latest.isError ? undefined : latest.data?.summary;
  if (!summary) {
    return null;
  }
  return (
    <section
      className="transcript-summary"
      aria-label={t("transcriptread.summaryTitle")}
    >
      <p className="transcript-status">
        <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
        <span className="t-caption">{t("transcriptread.summaryTitle")}</span>
      </p>
      <SummaryPoints
        title={t("transcriptread.keyPoints")}
        points={summary.key_points}
      />
      {summary.next_steps.length > 0 && (
        <SummaryPoints
          title={t("transcriptread.nextSteps")}
          points={summary.next_steps}
        />
      )}
    </section>
  );
}

function SummaryPoints({
  title,
  points,
}: Readonly<{ title: string; points: TranscriptSummaryPoint[] }>) {
  const plural = usePlural();
  return (
    <>
      <Eyebrow as="h3">{title}</Eyebrow>
      <ul className="transcript-summary-points">
        {points.map((point) => (
          <li key={`${point.source_lines.join(",")}:${point.text}`}>
            {point.text}{" "}
            <span className="t-caption">
              {plural("trust.evidenceLine", point.source_lines.length, {
                lines: formatSourceLines(point.source_lines),
              })}
            </span>
          </li>
        ))}
      </ul>
    </>
  );
}
