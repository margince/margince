import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, Button, Disclosure } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import type {
  ResolveAnswer,
  ResolveSheetLabels,
} from "../../design-system/resolvesheet";
import { formatDate, formatMoneyOrAbsent } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import { throwProblem, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { type AnalyticsScope, scopeQuery } from "./analytics.context";
import { EnforcementSwitch } from "./analytics.forecast.enforcement";
import { FirstCheck, Recheck } from "./analytics.forecast.firstcheck";
import {
  FreshnessLine,
  OutOfDateBadge,
  outOfDate,
  verdictStands,
} from "./analytics.forecast.freshness";
import { InputCheckAnswerSheet } from "./inputcheck.answer";
import { checkLabel, SubjectCell } from "./inputcheck.facts";

type InputCheck = components["schemas"]["InputCheck"];
type Assurance = components["schemas"]["ForecastAssurance"];
type Resolution = {
  id: string;
  answer: ResolveAnswer;
  dealId?: string;
  // A customer review's wait and lost answers also write the deal, through
  // their own routes.
  review?: boolean;
  // The reason a review closed lost is lost for, from the loss reason list.
  lostReasonCode?: string;
};

// What should be checked before the call.
//
// Two different statements sit here and must not be read as one. The RECORD
// problems are things the pipeline got wrong; the COVERAGE line is how much of
// the pipeline could be looked at. A run that could not open the mailbox and
// found nothing is not a clean pipeline, and folding the two into one count is
// the misreading this panel exists to prevent.
export function ForecastReview() {
  const t = useT();
  const { locale } = useLocale();

  const assurance = useQuery({
    queryKey: ["forecast-assurance"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/forecast/assurance",
        {},
      );
      // 404 is this endpoint's ANSWER, not its failure: the contract says "no
      // run has completed yet. A fresh installation has not been checked."
      // Thrown as a problem it reached the error gate and the panel read
      // "Couldn't load this view. not found" — which tells a reader the screen
      // is broken when what happened is that nothing has looked yet, and those
      // are opposite instructions about whether to trust the numbers above.
      if (response.status === 404) {
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data ?? null;
    },
  });

  return (
    <QueryGate query={assurance} pendingLabel={t("review.title")}>
      {(run) =>
        run === null ? (
          // 404 here is a workspace nobody has started, not a wait: the nightly
          // pass skips it until somebody does. So the panel ASKS, with the
          // preview that makes the ask answerable.
          <FirstCheck title={t("review.title")} />
        ) : (
          <ReviewPanel run={run} locale={locale} title={t("review.title")} />
        )
      }
    </QueryGate>
  );
}

// The open findings this reader may see, under the one key every reader of
// them shares, so answering one here clears it everywhere it is counted.
// The findings, for one record scope when given; without one, every finding
// the reader may see, which is what the forecast review lists.
export function useInputChecks(enabled = true, scope?: AnalyticsScope) {
  const query = scope ? scopeQuery(scope) : {};
  return useQuery({
    enabled,
    queryKey: ["input-checks", query],
    // The signal stops the read of a scope the reader has already left.
    queryFn: async ({ signal }) => {
      const data = unwrap(
        await api.GET("/forecast/assurance/exceptions", {
          params: { query },
          signal,
        }),
      );
      return data.data;
    },
  });
}

function ReviewPanel({
  run,
  locale,
  title,
}: Readonly<{ run: Assurance; locale: Locale; title: string }>) {
  const t = useT();

  const checks = useInputChecks();

  return (
    <Panel
      title={title}
      titleAction={
        <>
          {outOfDate(run) && <OutOfDateBadge />}
          <ReadinessBadge run={run} />
        </>
      }
    >
      <PanelBody>
        <FreshnessLine freshness={run.freshness} />
        {run.sources?.some((source) => source.state !== "checked") ? (
          <CoverageLine sources={run.sources} />
        ) : (
          <Disclosure summary={t("analytics.sectionCoverage")}>
            <CoverageLine sources={run.sources} />
          </Disclosure>
        )}
        {/* Beside the coverage line rather than in the header: a recheck is an
            answer to what the coverage line just said, and the header already
            carries the verdict. */}
        <Recheck runId={run.run_id} />
        <EnforcementSwitch />
      </PanelBody>
      <QueryGate query={checks} pendingLabel={title}>
        {(found) =>
          found.length === 0 ? (
            <PanelBody>
              <p className="sub">{t("review.nothingToCheck")}</p>
            </PanelBody>
          ) : (
            <CheckTable checks={found} locale={locale} title={title} />
          )
        }
      </QueryGate>
    </Panel>
  );
}

// The verdict, as a word rather than a count.
//
// `checks_incomplete` is not a worse `needs_review`. One says the pipeline has
// problems, the other says we could not look, and they ask for different
// things. A verdict on records that changed since, or may have, keeps its
// word and loses its tone. "Ready" in green over a moved pipeline would be a
// false healthy reading.
function ReadinessBadge({ run }: Readonly<{ run: Assurance }>) {
  const t = useT();
  if (!run.readiness) {
    return null;
  }

  const tone = {
    ready: "success",
    ready_with_exceptions: "accent",
    needs_review: "warning",
    checks_incomplete: "warning",
  } as const;
  const label = {
    ready: "review.ready",
    ready_with_exceptions: "review.readyWithExceptions",
    needs_review: "review.needsReview",
    checks_incomplete: "review.checksIncomplete",
  } as const;
  return (
    <Badge tone={verdictStands(run) ? tone[run.readiness] : "default"}>
      {t(label[run.readiness])}
    </Badge>
  );
}

// How much of the pipeline the run could read.
//
// Named sources rather than a count: "2 of 6 sources" tells a reader a number,
// and which two is what they need to fix it.
export function CoverageLine({
  sources,
}: Readonly<{ sources: Assurance["sources"] | undefined }>) {
  const t = useT();
  const unread = (sources ?? []).filter((source) => source.state !== "checked");
  if (unread.length === 0) {
    return <p className="sub">{t("review.allSourcesRead")}</p>;
  }
  return (
    <p className="sub">
      {t("review.sourcesUnread", {
        sources: unread
          .map((source) => sourceName(source.source, t))
          .join(", "),
      })}
    </p>
  );
}

// A source's name in the reader's language.
//
// The wire carries the server's own vocabulary — "mail", "offers" — and the
// line printed it verbatim, so a German reader was told which sources went
// unread in English, in words that name a table rather than a thing they
// recognise. An unknown source falls back to its wire key: a source this
// release has not heard of is better named badly than not named at all, since
// the whole point of the line is which one to go and fix.
// Exported for the Data coverage table, which draws the same source enum:
// two screens naming one vocabulary share one translator.
export function sourceName(source: string, t: ReturnType<typeof useT>): string {
  switch (source) {
    case "mail":
      return t("review.source.mail");
    case "offers":
      return t("review.source.offers");
    case "calendar":
      return t("review.source.calendar");
    case "documents":
      return t("review.source.documents");
    case "contracts":
      return t("review.source.contracts");
    case "incumbent":
      return t("review.source.incumbent");
    default:
      return source;
  }
}

// The findings, and the one sheet that answers whichever was picked.
//
// The order is the server's: severity first, then money at stake, which is the
// endpoint's stated contract. Re-sorting here would be a second opinion on it.
function CheckTable({
  checks,
  locale,
  title,
}: Readonly<{ checks: InputCheck[]; locale: Locale; title: string }>) {
  const t = useT();
  const zone = useRecordZone();
  // Closing keeps `id`, so the sheet's key holds and the Modal closes and
  // returns focus normally; a DIFFERENT finding changes the key and starts blank.
  const [sheet, setSheet] = useState<{ id: string; open: boolean } | null>(
    null,
  );
  const table = useRef<HTMLDivElement>(null);
  const opener = useRef<{ button: HTMLElement; row: number } | null>(null);
  const close = () => setSheet((was) => was && { ...was, open: false });
  // A save outlives its sheet: the reader may cancel, open another finding and
  // still be answering it when the first save lands, so only its own sheet closes.
  const resolve = useResolveCheck((id) =>
    setSheet((was) => (was?.id === id ? { ...was, open: false } : was)),
  );
  // A finding a refetch took away is no longer the server's to answer, so the
  // sheet follows the list rather than the click that opened it.
  const open =
    sheet?.open === true && checks.some((check) => check.id === sheet.id);

  const answerButtons = (): HTMLElement[] => [
    ...(table.current?.querySelectorAll<HTMLElement>(".cell-actions button") ??
      []),
  ];
  const answer = (check: InputCheck, button: HTMLElement) => {
    opener.current = { button, row: answerButtons().indexOf(button) };
    resolve.reset();
    setSheet({ id: check.id, open: true });
  };
  // An answered finding leaves the list with its own button. The Answer now at
  // its row index is the FOLLOWING finding's (the last one's when it was last),
  // so a keyboard reader moves down the list rather than back up it.
  const focusAfterClose = (): HTMLElement | null => {
    const from = opener.current;
    if (from === null || from.button.isConnected) {
      return from?.button ?? null;
    }
    const left = answerButtons();
    return left[Math.min(from.row, left.length - 1)] ?? null;
  };

  return (
    <>
      <PanelBody>
        <div ref={table}>
          <DataTable<InputCheck>
            label={title}
            rows={checks}
            rowKey={(check) => check.id}
            columns={checkColumns(t, locale, zone, answer)}
          />
        </div>
      </PanelBody>
      <InputCheckAnswerSheet
        check={checks.find((check) => check.id === sheet?.id)}
        key={sheet?.id}
        open={open}
        pending={resolve.isPending}
        error={resolve.error}
        labels={sheetLabels(t)}
        returnFocusTo={focusAfterClose}
        onSubmit={(given, dealId, lostReasonCode) => {
          if (sheet !== null) {
            const review =
              checks.find((check) => check.id === sheet.id)?.type ===
              "deal_viability";
            resolve.mutate({
              id: sheet.id,
              answer: given,
              dealId,
              review,
              lostReasonCode,
            });
          }
        }}
        onClose={close}
      />
    </>
  );
}

function checkColumns(
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
  answer: (check: InputCheck, button: HTMLElement) => void,
): DataTableColumn<InputCheck>[] {
  return [
    {
      key: "severity",
      header: t("review.colSeverity"),
      render: (check) => <SeverityBadge severity={check.severity} />,
    },
    {
      key: "finding",
      header: t("review.colFinding"),
      render: (check) => (
        <>
          {t(checkLabel(check.type))}{" "}
          {check.inputs_changed && <OutOfDateBadge />}
        </>
      ),
    },
    {
      key: "deal",
      header: t("review.colDeal"),
      render: (check) => <SubjectCell check={check} />,
    },
    {
      key: "stake",
      header: t("review.colAtStake"),
      render: (check) => (
        <span className="t-num">
          {formatMoneyOrAbsent(
            check.affected_minor ?? null,
            check.currency ?? "",
            locale,
          )}
        </span>
      ),
    },
    {
      key: "since",
      // The record's clock, not the reader's: colleagues quote this date to
      // each other, and two of them must read the same day.
      header: t("review.colSeenSince"),
      render: (check) => formatDate(check.first_seen_at, locale, zone),
    },
    {
      key: "answer",
      header: t("table.actions"),
      headerHidden: true,
      render: (check) => (
        <div className="cell-actions">
          <Button onClick={(event) => answer(check, event.currentTarget)}>
            {t("review.answer")}
          </Button>
        </div>
      ),
    },
  ];
}

// The answer to one finding, sent.
//
// The finding and the answer travel as VARIABLES. Read from the closure they
// would be whatever the last render saw, which is the wrong finding exactly
// when a save races a refetch.
function useResolveCheck(onResolved: (id: string) => void) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      answer,
      dealId,
      review,
      lostReasonCode,
    }: Resolution) => {
      if (review && answer.outcome === "remind_later") {
        unwrap(
          await api.POST("/forecast/assurance/exceptions/{id}/wait", {
            params: { path: { id } },
            body: { until: answer.remindAt ?? "" },
          }),
        );
        return;
      }
      if (review && answer.outcome === "not_relevant") {
        unwrap(
          await api.POST("/forecast/assurance/exceptions/{id}/close-lost", {
            params: { path: { id } },
            body: {
              lost_reason: answer.reason ?? "",
              lost_reason_code: lostReasonCode,
            },
          }),
        );
        return;
      }
      unwrap(
        await api.POST("/forecast/assurance/exceptions/{id}/resolve", {
          params: { path: { id } },
          body: {
            outcome: answer.outcome,
            deal_id: answer.outcome === "fixed_record" ? dealId : undefined,
            reason: answer.reason,
            remind_at: answer.remindAt,
            expires_at: answer.expiresAt,
          },
        }),
      );
    },
    onSuccess: async (_data, { id }) => {
      // Both lists move: the finding leaves this one, and the run's readiness
      // may change with it.
      await client.invalidateQueries({ queryKey: ["input-checks"] });
      await client.invalidateQueries({ queryKey: ["forecast-assurance"] });
      onResolved(id);
    },
  });
}

// How urgent a finding is, as a word in the tone the rest of the product gives
// that urgency.
export function SeverityBadge({
  severity,
}: Readonly<{ severity: InputCheck["severity"] }>) {
  const t = useT();
  const tone = { high: "danger", medium: "warning", low: "default" } as const;
  const label = {
    high: "review.severityHigh",
    medium: "review.severityMedium",
    low: "review.severityLow",
  } as const;
  return <Badge tone={tone[severity]}>{t(label[severity])}</Badge>;
}
export function sheetLabels(t: ReturnType<typeof useT>): ResolveSheetLabels {
  return {
    title: t("review.sheetTitle"),
    outcomeLegend: t("review.outcomeLegend"),
    outcomes: [
      { value: "fixed_record", label: t("review.fixedRecord") },
      { value: "added_evidence", label: t("review.addedEvidence") },
      {
        value: "value_correct",
        label: t("review.valueCorrect"),
        description: t("review.hidesUntilExpiry"),
      },
      {
        value: "not_relevant",
        label: t("review.notRelevant"),
        description: t("review.hidesUntilExpiry"),
      },
      { value: "remind_later", label: t("review.remindLater") },
      { value: "reassign", label: t("review.reassign") },
    ],
    reason: t("review.reason"),
    reasonHelp: t("review.reasonHelp"),
    remindAt: t("review.remindAt"),
    expiresAt: t("review.expiresAt"),
    expiresHelp: t("review.expiresHelp"),
    cancel: t("review.cancel"),
    submit: t("review.submit"),
  };
}
