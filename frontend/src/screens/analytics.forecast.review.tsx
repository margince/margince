import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import {
  type ResolveAnswer,
  ResolveSheet,
  type ResolveSheetLabels,
} from "../design-system/resolvesheet";
import { formatMoneyOrAbsent } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";

type InputCheck = components["schemas"]["InputCheck"];
type Assurance = components["schemas"]["ForecastAssurance"];

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
            found.map((check) => (
              <CheckRow key={check.id} check={check} locale={locale} />
            ))
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
    needs_review: "warn",
    checks_incomplete: "warn",
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

// One finding, and the way to answer it.
function CheckRow({
  check,
  locale,
}: Readonly<{ check: InputCheck; locale: Locale }>) {
  const t = useT();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);

  const resolve = useMutation({
    // The answer travels as a VARIABLE. Read from the closure it would be
    // whatever the last render saw, which is the wrong answer exactly when a
    // save races a refetch.
    mutationFn: async (answer: ResolveAnswer) => {
      const { error } = await api.POST(
        "/forecast/assurance/exceptions/{id}/resolve",
        {
          params: { path: { id: check.id } },
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
    onSuccess: async () => {
      // Both lists move: the finding leaves this one, and the run's readiness
      // may change with it.
      await client.invalidateQueries({ queryKey: ["input-checks"] });
      await client.invalidateQueries({ queryKey: ["forecast-assurance"] });
      setOpen(false);
    },
  });

  return (
    <>
      <PanelRow>
        <span>{t(checkLabel(check.type))}</span>
        <span className="num">
          {formatMoneyOrAbsent(
            check.affected_minor ?? null,
            check.currency ?? "",
            locale,
          )}
        </span>
        <Button small onClick={() => setOpen(true)}>
          {t("review.answer")}
        </Button>
      </PanelRow>
      <ResolveSheet
        open={open}
        pending={resolve.isPending}
        labels={sheetLabels(t)}
        onSubmit={(answer) => resolve.mutate(answer)}
        onClose={() => setOpen(false)}
      />
    </>
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
