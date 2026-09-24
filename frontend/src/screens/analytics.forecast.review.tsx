import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { isEntityKind } from "../app/entity";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody } from "../design-system/panel";
import {
  type ResolveAnswer,
  ResolveSheet,
  type ResolveSheetLabels,
} from "../design-system/resolvesheet";
import { formatDate, formatMoneyOrAbsent } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";
import { EntityRef } from "./entityref";

type InputCheck = components["schemas"]["InputCheck"];
type Assurance = components["schemas"]["ForecastAssurance"];
type Resolution = { id: string; answer: ResolveAnswer };
type CheckColumn = {
  key: string;
  header: string;
  render: (check: InputCheck) => ReactNode;
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
          <Panel title={t("review.title")}>
            <PanelBody>
              {/* Said plainly, because the alternative reading is dangerous: a
                  reader who takes an unchecked pipeline for a clean one has
                  been told the opposite of what happened. */}
              <p>{t("review.notCheckedYet")}</p>
            </PanelBody>
          </Panel>
        ) : (
          <ReviewPanel run={run} locale={locale} title={t("review.title")} />
        )
      }
    </QueryGate>
  );
}

function ReviewPanel({
  run,
  locale,
  title,
}: Readonly<{ run: Assurance; locale: Locale; title: string }>) {
  const t = useT();

  const checks = useQuery({
    queryKey: ["input-checks"],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/forecast/assurance/exceptions",
        {},
      );
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });

  return (
    <Panel title={title} titleAction={<ReadinessBadge run={run} />}>
      <PanelBody>
        {/* Coverage first and SEPARATE. A reader who takes "no findings" for a
            clean pipeline when nobody could look has been told the opposite of
            what happened. */}
        <CoverageLine run={run} />
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
// `checks_incomplete` is not a worse `needs_review`: one says the pipeline has
// problems, the other says we could not look, and they take different tones
// because they ask for different things.
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
  return <Badge tone={tone[run.readiness]}>{t(label[run.readiness])}</Badge>;
}

// How much of the pipeline the run could read.
//
// Named sources rather than a count: "2 of 6 sources" tells a reader a number,
// and which two is what they need to fix it.
function CoverageLine({ run }: Readonly<{ run: Assurance }>) {
  const t = useT();
  const unread = (run.sources ?? []).filter(
    (source) => source.state !== "checked",
  );
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
      <ResolveSheet
        key={sheet?.id}
        open={open}
        pending={resolve.isPending}
        error={resolve.error}
        labels={sheetLabels(t)}
        returnFocusTo={focusAfterClose}
        onSubmit={(given) => {
          if (sheet !== null) {
            resolve.mutate({ id: sheet.id, answer: given });
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
): CheckColumn[] {
  return [
    {
      key: "severity",
      header: t("review.colSeverity"),
      render: (check) => <SeverityBadge severity={check.severity} />,
    },
    {
      key: "finding",
      header: t("review.colFinding"),
      render: (check) => t(checkLabel(check.type)),
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
    mutationFn: async ({ id, answer }: Resolution) => {
      const { error } = await api.POST(
        "/forecast/assurance/exceptions/{id}/resolve",
        {
          params: { path: { id } },
          body: {
            outcome: answer.outcome,
            reason: answer.reason,
            remind_at: answer.remindAt,
            expires_at: answer.expiresAt,
          },
        },
      );
      if (error) {
        throwProblem(error);
      }
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
function SeverityBadge({
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

// WHICH deal. Without it a manager reading the review before a call knew
// something was wrong and not what it was wrong about. An older server sends no
// subject, and a kind the registry has no screen for is left out the same way
// the Worklist's own rows leave it: the cell stays empty.
function SubjectCell({ check }: Readonly<{ check: InputCheck }>) {
  if (!check.subject || !isEntityKind(check.subject.type)) {
    return null;
  }
  return (
    <EntityRef
      kind={check.subject.type}
      id={check.subject.id}
      name={check.subject.label}
    />
  );
}

// A finding's own name, keyed by the check that noticed it.
//
// An unknown type falls back to a generic line rather than rendering the raw
// key: a server that grew a tenth rule before this screen learned its name
// should show a finding a reader can still act on, not `close_pushed_v2`.
function checkLabel(type: string): Parameters<ReturnType<typeof useT>>[0] {
  const known: Record<string, string> = {
    close_past: "review.closePast",
    close_unconfirmed: "review.closeUnconfirmed",
    close_pushed: "review.closePushed",
    amount_vs_offer: "review.amountVsOffer",
    amount_vs_contract: "review.amountVsContract",
    no_next_step: "review.noNextStep",
    no_economic_buyer: "review.noEconomicBuyer",
    buyer_silent: "review.buyerSilent",
    commit_unpriced: "review.commitUnpriced",
  };
  return (known[type] ?? "review.unknownCheck") as Parameters<
    ReturnType<typeof useT>
  >[0];
}

function sheetLabels(t: ReturnType<typeof useT>): ResolveSheetLabels {
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
