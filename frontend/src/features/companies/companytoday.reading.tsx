import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AiPending } from "../../design-system/aipending";
import { PanelRow } from "../../design-system/panel";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import {
  FoundMove,
  type Grounding,
  momentIsARow,
  momentKicker,
  type OpenReceipt,
  type StandingTone,
  WithheldNotice,
} from "../records/record360";
import { type AccountScan, scanIsLive } from "./accountscan";
import {
  nextCommitmentLine,
  type SuggestionAction,
  useSuggestionsBody,
} from "./company360";
import {
  HEALTH_DIMENSION_LABEL,
  HEALTH_DIMENSION_MEANS,
  HEALTH_RATING_LABEL,
  HEALTH_STANDING_TONE,
  useAccountStanding,
} from "./companylookups";
import type { Company360 } from "./companytoday";
import {
  briefFooter,
  leadSentence,
  manualMoveRows,
  omitted,
  scanFoot,
  TODAY_SOURCES,
  whoseMove,
} from "./companytoday.lines";
import {
  momentBasis,
  momentDefer,
  momentFallbackVerb,
  momentVerb,
} from "./companytodayverbs";
import "./company360.css";

type HealthRating = components["schemas"]["HealthDimension"]["rating"];

/** One of the three rated dimensions under the verdict. */
export type TodayDimension = {
  key: "relationship" | "commercial" | "payment";
  // What is rated.
  label: string;
  // The rating in words, or where there is none, why: nothing read yet, or
  // nothing shown to this reader. Never absent, because a blank chip would
  // read as a rating of zero.
  reading: string;
  // How loud the rating is. Absent where there is no rating to be loud about.
  tone?: "calm" | "warning" | "danger";
  // What this dimension weighs. Three words on a card cannot say what went
  // into "Commercial · Good", and a rating nobody can interpret is taken on trust.
  means: string;
  // The reading the rating was made from, in the rule's own words. Absent
  // where nothing was rated, since there is then no working to show.
  because?: string;
};

/**
 * The account's reading for today, computed once and drawn in two panes. One
 * is the 360's call: the word, the sentence it rests on, the dimensions. The
 * other is the needs list: the moment, the agent's finds, the manual moves.
 * Two panes, because the glance puts them in different cells of the page.
 * One reading, because the verdict and the queue come off the same hooks.
 * Two computations of "how is this account" would drift apart.
 */
export type TodayReading =
  | { state: "loading" }
  | { state: "failed" }
  | {
      state: "ready";
      standing: { label: string; tone: StandingTone };
      because?: ReactNode;
      restsOn: readonly Grounding[];
      dimensions: readonly TodayDimension[];
      rows: ReactNode[];
      // False when a source the rows are read from was withheld.
      complete: boolean;
      footer?: ReactNode;
      notice?: ReactNode;
    };

// What a reading is computed from: the account's composite read and the
// verbs the surface above can perform on what it says.
export type TodayReadingInputs = Readonly<{
  companyId: string;
  view?: Company360;
  loading: boolean;
  // The composite read failed. Distinct from "still loading" and from "nothing
  // is happening on this account". All three draw a short section, and only
  // one of them is a fact about the account.
  failed: boolean;
  // Opens the pre-meeting brief for the day's meeting.
  onPrepareMeeting?: (activityId: string) => void;
  // Opens the composer anchored on the account and its recipient.
  onDraftTo?: (contactId: string) => void;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Opens a cited receipt or mail in the page's drawers. A suggestion resting
  // on an unanswered mail names that mail, and reading it is the reader's move.
  onOpenReceipt?: OpenReceipt;
  onOpenEmail?: (activityId: string) => void;
  // Performing a suggestion's own action. The composer, the deal and the
  // task form all live above this brief.
  onPerform?: (action: SuggestionAction) => void;
  // Opens a named task's detail modal on the Tasks tab, where it lives. It is
  // the leading card's fallback when the moment names no record to open.
  onOpenTask?: (activityId: string) => void;
  // The header's Log-activity drawer: the leading card's second verb.
  onLogActivity?: () => void;
  // The reader's scan of the account, when the page holds one. Its merged
  // advice replaces the 360's own rows, and a read in flight draws the pending
  // row. The foot says who read what and when.
  scan?: AccountScan;
}>;

export function useTodayReading({
  companyId,
  view,
  loading,
  failed,
  onPrepareMeeting,
  onDraftTo,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  onPerform,
  onOpenTask,
  onLogActivity,
  scan,
}: TodayReadingInputs): TodayReading {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // Called before the loading/failed branches below, like every other hook
  // here: React requires it, and it answers "nothing rated" on its own.
  const verdict = useAccountStanding(companyId, view?.health);
  const suggestions = useSuggestionsBody({
    companyId,
    view,
    onOpenRecord,
    onOpenReceipt,
    onOpenEmail,
    onPerform,
    advice: scan
      ? { findings: scan.findings, dropped: scan.findings_dropped }
      : undefined,
  });
  if (loading) {
    return { state: "loading" };
  }
  if (failed || !view) {
    return { state: "failed" };
  }
  const lead = whoseMove({ view, t, locale });
  const commitment = nextCommitmentLine(view, locale, t);
  // Nothing rated is itself a standing: an account too new to score reads
  // "not assessed". The sentence under the call still has a call to belong to. The words match the readings row's own, so the two cannot disagree.
  // A withheld health section is a fact about the reader; nothing rated is a
  // fact about how much has been judged. Shared words would send a reader to
  // ask for a grant that shows them nothing.
  const healthWithheld = omitted(view, "health");
  const standing = verdict.overall
    ? {
        label: t(HEALTH_RATING_LABEL[verdict.overall]),
        tone: HEALTH_STANDING_TONE[verdict.overall],
      }
    : {
        label: t(healthWithheld ? "record.notShown" : "co.strip.notAssessed"),
        tone: "unknown" as const,
      };
  const rated: ReadonlyArray<
    [TodayDimension["key"], HealthRating | undefined]
  > = [
    ["relationship", view.health?.relationship?.rating],
    ["commercial", view.health?.commercial?.rating],
    ["payment", verdict.payment?.rating],
  ];
  const dimensions: TodayDimension[] = rated.map(([key, rating]) => ({
    key,
    label: t(HEALTH_DIMENSION_LABEL[key]),
    reading: rating
      ? t(HEALTH_RATING_LABEL[rating])
      : t(healthWithheld ? "record.notShown" : "co.strip.notAssessed"),
    tone: rating ? HEALTH_STANDING_TONE[rating] : undefined,
    means: t(HEALTH_DIMENSION_MEANS[key]),
    // The working, from the same place the call's own grounding is built.
    // So a dimension's chip and the verdict's "what this rests on" cannot
    // quote two different readings of one dimension.
    because: verdict.restsOn.find((reading) => reading.key === key)?.quote,
  }));
  // The read in flight, above the rows it will add to. The rules' rows stand
  // while Margince reads, and the pending row says more is coming.
  const scanRows: ReactNode[] = scanIsLive(scan)
    ? [
        <PanelRow key="scan" className="co-move co-move-reading">
          <AiPending
            label={t(
              scan?.state === "running"
                ? "today.scan.reading"
                : "today.scan.queued",
            )}
            lines={2}
          />
        </PanelRow>,
      ]
    : [];
  const manual = manualMoveRows({
    view,
    t,
    onPrepareMeeting,
    onDraftTo,
    hasDraftReply: suggestions.hasDraftReply,
  });
  // How much the list holds without the moment, which decides whether the
  // quiet card is still an answer (`momentIsARow`). Counted rather than
  // measured off the rows below: `suggestions.rows` is one node carrying
  // several, so the section reports its own count.
  const besidesTheMoment = scanRows.length + suggestions.count + manual.length;
  // The moment's own verb only where the server named a destination for it.
  // Otherwise this fallback still gives the card that says what is owed a verb.
  const momentAction = momentFallbackVerb({
    view,
    t,
    onOpenTask,
    onDraftTo,
    onLogActivity,
  });
  // What we owe leads the list. A promise past its date outranks a reading of
  // the account: one is a thing to do today and the other is context for it.
  const rows: ReactNode[] = [
    // The same row the contact page leads with, drawn by the same component.
    // So two records of one account spell "here is the move" one way. The quiet
    // rung is a reading rather than a find, so it carries no byline or rule.
    ...(view.moment && momentIsARow(view.moment, besidesTheMoment > 0)
      ? [
          <FoundMove
            key="moment"
            suggested={view.moment.rule !== "nothing_needed"}
            title={view.moment.headline}
            why={view.moment.why_now}
            kicker={momentKicker(view.moment, t)}
            basis={momentBasis(view.moment)}
            action={momentVerb({
              moment: view.moment,
              onOpenRecord,
              fallback: momentAction,
            })}
            defer={momentDefer(view.moment, suggestions.deferOf)}
          />,
        ]
      : []),
    ...scanRows,
    suggestions.rows,
    ...manual,
  ];
  const withheld = TODAY_SOURCES.filter((source) =>
    omitted(view, source.section),
  );
  return {
    state: "ready",
    standing,
    because: lead ? leadSentence(lead) : undefined,
    restsOn: verdict.restsOn,
    dimensions,
    rows,
    complete: withheld.length === 0,
    footer: briefFooter(
      commitment,
      suggestions.footer,
      scanFoot({ scan, t, plural, locale, recordZone }),
    ),
    notice: (
      <WithheldNotice sections={withheld.map((source) => t(source.label))} />
    ),
  };
}
