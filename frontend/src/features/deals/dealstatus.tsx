import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { HowItStands } from "../../design-system/howitstands";
import { NextBar, type NextBarMore } from "../../design-system/nextbar";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  calendarDaysBetween,
  formatDayMonth,
  formatNumber,
} from "../../format/format";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { hasMoveControl, MoveButton } from "../records/movebutton";
import {
  Citations,
  openCitation,
  SentenceList,
  SignalStrip,
  useSetAside,
  WrittenBy,
} from "../records/record360";
import type { DealStandingHead } from "./deal360/dealhowitstands";
import { dealVerdictTone, verdictSourceCount } from "./deal360/dealstanding";
import { waitingSince } from "./deal360/dealwaiting";
import { useDealSignals } from "./dealsignals";
import "./dealstatus.css";

// The deal page's reading, read before a call. The next bar is the one move
// the status card names, with the verb that performs it. How it stands is the
// call the server reached and its first line of reasoning, with its receipts.
// The brief is drawn in the board's drawer only. It says what has happened and
// what the buyer wants, in prose with its sources, the rest behind a fold.
//
// The read writes nothing a user can see: the server caches the card per
// reader, and a changed deal is rewritten before the read answers. The click
// performs the move through the verb the card names, so nothing here
// re-implements a write or skips its gates.

type DealStatusCard = components["schemas"]["DealStatusCard"];
type DealStatusCardMove = components["schemas"]["DealStatusCardMove"];
type DealStatusCardSection = components["schemas"]["DealStatusCardSection"];
type Activity = components["schemas"]["Activity"];
type DealRef = Readonly<{ id: string; name: string; stalled?: boolean }>;

// The verdict words the server may send. A word this build does not know
// renders as itself: a fifth arriving from a newer server is still a call the
// card must show.
const VERDICT_LABELS: Record<string, MessageKey> = {
  live: "deal360.verdict.live",
  drifting: "deal360.verdict.drifting",
  blocked: "deal360.verdict.blocked",
  cold: "deal360.verdict.cold",
};

// The card's read, shared. Deal360 draws the briefing from it and the email
// box takes `reply_to` out of the same entry, so the two cannot end up
// disagreeing about whether there is a message waiting for an answer — and the
// box costs no second request.
//
// `live` is false for an archived deal, whose status the server refuses.
export function useDealStatusCard(dealId: string, live: boolean) {
  const t = useT();
  return useQuery({
    queryKey: ["deal-status", dealId],
    enabled: live,
    queryFn: async () => {
      return unwrap(
        await api.GET("/deals/{id}/status", {
          params: { path: { id: dealId }, query: { facts_only: true } },
        }),
        t,
      );
    },
  });
}

/**
 * The deal's reading on its Overview: the next move as a bar, How it stands,
 * then the written brief. How it stands is drawn in every state of the read,
 * because the stage ladder it carries is how the deal is moved. While the
 * card is pending or unreadable it says so in its own words.
 */
export function DealStatusCardPanel({
  deal,
  renderStanding,
  onOpenEmail,
  timeline = [],
  asOf,
  more,
}: Readonly<{
  deal: DealRef;
  // How it stands, drawn by the page that holds the ladder's write path.
  // Absent, the card draws the verdict alone, as the board's drawer does.
  renderStanding?: (head: DealStandingHead) => ReactNode;
  // Opens a cited message in the deal page's own email drawer.
  onOpenEmail?: (activityId: string) => void;
  // The page's timeline rows, to date the message a reply is owed to.
  timeline?: readonly Activity[];
  // The instant the page was read at, so the Next bar counts the wait to the
  // same moment the figures under it do. Absent, the card's own instant.
  asOf?: string;
  // The other open tasks, for the Next bar's tail.
  more?: NextBarMore;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const status = useDealStatusCard(deal.id, true);
  const rewrite = useMutation({
    mutationKey: ["deal-status-refresh", deal.id],
    mutationFn: async () => {
      return unwrap(
        await api.GET("/deals/{id}/status", {
          params: { path: { id: deal.id }, query: { refresh: true } },
        }),
        t,
      );
    },
    onSuccess: (data) =>
      queryClient.setQueryData(["deal-status", deal.id], data),
  });
  const card = status.data;
  const standing =
    renderStanding ?? ((head: DealStandingHead) => <VerdictOnly head={head} />);
  if (card?.story) {
    return (
      <>
        <DealNextBar
          dealId={deal.id}
          card={card}
          timeline={timeline}
          asOf={asOf ?? card.generated_at}
          onOpenEmail={onOpenEmail}
          more={more}
        />
        {standing(cardHead(card, deal, t, onOpenEmail))}
        {/* The record page says what the brief says through How it stands and
            its cards; the board's drawer has neither, so only it reads the
            brief. */}
        {renderStanding ? null : (
          <Briefing
            dealId={deal.id}
            card={card}
            onOpenEmail={onOpenEmail}
            onRewrite={() => rewrite.mutate()}
            rewriting={rewrite.isPending}
          />
        )}
      </>
    );
  }
  // `story` is required on the wire, so a loaded card without one is not the
  // shape the contract promises; saying so beats a verdict nobody reached.
  return (
    <>
      {standing(
        unreadHead(
          deal,
          t,
          status.isSuccess ? t("deal360.unreadable") : undefined,
        ),
      )}
      {status.isError && <ErrorLine error={status.error} />}
    </>
  );
}

// The verdict without the deal page's ladder and figures. A read with no
// instant of its own has no day to head the card with, so it draws nothing.
function VerdictOnly({ head }: Readonly<{ head: DealStandingHead }>) {
  const { locale } = useLocale();
  const zone = useRecordZone();
  if (!head.readAt) {
    return null;
  }
  return (
    <HowItStands
      date={formatDayMonth(head.readAt, locale, zone)}
      verdict={head.verdict}
      sources={head.sources}
    />
  );
}

/** The head of How it stands when no card is on hand: the archived deal, a
 *  read in flight or one that failed. Nothing model-written is claimed. */
export function unreadHead(
  deal: Pick<DealRef, "stalled">,
  t: Translator,
  sentence?: string,
): DealStandingHead {
  return {
    verdict: deal.stalled
      ? { word: t("deal.stalledBadge"), tone: "warning", sentence }
      : { word: t("standing.notAssessed"), tone: "neutral", sentence },
    sources: { by: "records" },
  };
}

function cardHead(
  card: DealStatusCard,
  deal: Pick<DealRef, "stalled">,
  t: Translator,
  onOpenEmail?: (activityId: string) => void,
): DealStandingHead {
  if (!card.verdict) {
    return { ...unreadHead(deal, t), readAt: card.generated_at };
  }
  const [lead] = card.verdict.because.sentences;
  return {
    verdict: {
      word: verdictLabel(card.verdict.standing, t),
      tone: dealVerdictTone(card.verdict.standing),
      // The lead line keeps its receipts: it is the sentence a reader is most
      // likely to challenge.
      sentence: lead ? (
        <>
          {lead.text}
          <Citations
            evidence={lead.evidence}
            onOpenRecord={openCitation}
            onOpenEmail={onOpenEmail}
          />
        </>
      ) : undefined,
    },
    // The indigo pill claims a model wrote the call; the structured fallback
    // wrote this one when it did not.
    sources:
      card.generated_by === "model"
        ? { by: "model", count: verdictSourceCount(card) }
        : { by: "records" },
    readAt: card.generated_at,
  };
}

function Briefing({
  dealId,
  card,
  onOpenEmail,
  onRewrite,
  rewriting,
}: Readonly<{
  dealId: string;
  card: DealStatusCard;
  // Opens a cited message in the deal page's email drawer; see `Citations`.
  onOpenEmail?: (activityId: string) => void;
  onRewrite: () => void;
  rewriting: boolean;
}>) {
  const t = useT();
  // The findings ride the coverage card's own query, so this costs no second
  // request and the two cannot disagree about what is wrong with the deal.
  const coverage = useDealSignals(dealId);
  const because = card.verdict?.because.sentences ?? [];
  return (
    // What has happened and where that leaves things, in prose with its
    // sources. The blocker, the buyer and the rest of the reasoning sit behind
    // a fold, every word still cited.
    <Panel
      title={t("deal360.brief")}
      // A machine's reading in every state it can be in, so the tint rides
      // the panel. Which writer answered is sourcing, and sits in the foot
      // beside the verb that has it written again.
      tone="ai"
      titleAction={<Badge tone="ai">{t("co.assistant.aiTag")}</Badge>}
      footer={
        <div className="deal360-foot">
          <WrittenBy by={card.generated_by} />
          <Button variant="aiQuiet" pending={rewriting} onClick={onRewrite}>
            <RefreshCw aria-hidden />
            {t("deal360.rewrite")}
          </Button>
        </div>
      }
    >
      <SignalStrip signals={coverage.signals} />
      <Section
        section={card.story}
        onOpenRecord={openCitation}
        onOpenEmail={onOpenEmail}
        lead
      />
      <details className="deal360-fold">
        <summary>{t("deal360.readFull")}</summary>
        <Section
          heading={t("deal360.blocker")}
          section={card.blocker}
          onOpenRecord={openCitation}
          onOpenEmail={onOpenEmail}
          tone="warning"
        />
        <Section
          heading={t("deal360.buyer")}
          section={card.buyer}
          onOpenRecord={openCitation}
          onOpenEmail={onOpenEmail}
        />
        {/* Its first line is already the verdict's sentence. */}
        {because.length > 1 ? (
          <Section
            section={{ sentences: because.slice(1) }}
            onOpenRecord={openCitation}
            onOpenEmail={onOpenEmail}
          />
        ) : null}
      </details>
    </Panel>
  );
}

// Section renders one headed block, and renders NOTHING when the server sent
// none. An absent section means the records did not support saying anything —
// an empty "what is holding this up" would read as "nothing is".
function Section({
  heading,
  section,
  onOpenRecord,
  onOpenEmail,
  tone,
  lead,
}: Readonly<{
  heading?: string;
  section: DealStatusCardSection | undefined;
  onOpenRecord: (entityType: string, entityId: string) => void;
  // Opens a cited message; see `Citations`.
  onOpenEmail?: (activityId: string) => void;
  tone?: "warning";
  // The brief's opening block leads with its judgement, the way every other
  // written reading on a record does.
  lead?: boolean;
}>) {
  if (!section || section.sentences.length === 0) {
    return null;
  }
  return (
    <PanelBody>
      {heading ? (
        <p className={tone === "warning" ? "deal360-warning" : undefined}>
          {heading}
        </p>
      ) : null}
      <SentenceList
        sentences={section.sentences}
        onOpenRecord={onOpenRecord}
        onOpenEmail={onOpenEmail}
        leadWithJudgement={lead}
      />
    </PanelBody>
  );
}

// verdictLabel names a standing for a reader. A word this build does not know
// renders as itself rather than vanishing: the reader has learned four, and a
// fifth arriving from a newer server is still a call the card must show.
function verdictLabel(
  standing: string,
  t: (key: MessageKey) => string,
): string {
  const key = Object.hasOwn(VERDICT_LABELS, standing)
    ? VERDICT_LABELS[standing]
    : undefined;
  return key ? t(key) : standing;
}

// The move the briefing names, as the bar at the top of the Overview. The
// reason is the ask, the first line of evidence is why, and the verb performs
// it. A move with no verb of its own can still be snoozed.
function DealNextBar({
  dealId,
  card,
  timeline,
  asOf,
  onOpenEmail,
  more,
}: Readonly<{
  dealId: string;
  card: DealStatusCard;
  timeline: readonly Activity[];
  asOf: string;
  onOpenEmail?: (activityId: string) => void;
  more?: NextBarMore;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const notNow = useDealMoveSetAside(dealId);
  const move = card.next;
  if (!move) {
    return <NextBar more={more} />;
  }
  const late = replyLate(card, move, timeline, asOf, zone);
  // Only the message the buyer is owed an answer to opens in the email
  // drawer. A task or a meeting the move cites is not a message.
  const message = move.evidence.some(
    (row) => row.activity_id && row.activity_id === card.reply_to,
  )
    ? card.reply_to
    : undefined;
  const snooze = move.fingerprint ? notNow.deferOf(move.fingerprint) : null;
  return (
    <>
      <NextBar
        title={move.reason}
        late={
          late === undefined
            ? undefined
            : {
                label: plural("standing.days", late, {
                  count: formatNumber(late, locale),
                }),
                tone: "warning",
              }
        }
        sub={move.evidence[0]?.text}
        insight={
          message && onOpenEmail ? (
            <Button variant="link" onClick={() => onOpenEmail(message)}>
              {t("deal.standing.next.openMessage")}
            </Button>
          ) : undefined
        }
        action={
          hasMoveControl(move) ? (
            <MoveButton dealId={dealId} move={move} />
          ) : undefined
        }
        snooze={
          snooze
            ? { onSnooze: snooze.onDefer, pending: snooze.pending }
            : undefined
        }
        more={more}
      />
      {notNow.failure && <ErrorLine error={notNow.failure} />}
    </>
  );
}

// Days the buyer has waited, when the move is the answer they are owed; any
// other move is not late by that clock.
function replyLate(
  card: DealStatusCard,
  move: DealStatusCardMove,
  timeline: readonly Activity[],
  asOf: string,
  zone: string,
): number | undefined {
  const owed = move.evidence.some((row) => row.activity_id === card.reply_to);
  const waiting = owed ? waitingSince(card, timeline) : null;
  if (!waiting) {
    return undefined;
  }
  return calendarDaysBetween(new Date(waiting.at), new Date(asOf), zone);
}

// The deal card's "Not now": set aside for this reader until the card names a
// different move, with the Undo every record page's move carries.
function useDealMoveSetAside(dealId: string) {
  const t = useT();
  const client = useQueryClient();
  return useSetAside<string>({
    setAside: async (fingerprint) => {
      unwrap(
        await api.POST("/deals/{id}/move/dismiss", {
          params: { path: { id: dealId } },
          body: { fingerprint },
        }),
        t,
      );
    },
    restore: async (fingerprint) => {
      unwrap(
        await api.DELETE("/deals/{id}/move/dismiss", {
          params: { path: { id: dealId }, query: { fingerprint } },
        }),
        t,
      );
    },
    refresh: () =>
      client.invalidateQueries({ queryKey: ["deal-status", dealId] }),
    idOf: (fingerprint) => fingerprint,
  });
}
