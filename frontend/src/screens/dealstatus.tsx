import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ListChecks, RefreshCw, Sparkles } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import { useDealSignals } from "./dealsignals";
import { PersonMeetingBrief } from "./meetingbrief";
import {
  CallCard,
  FoundMove,
  SentenceList,
  SignalStrip,
  type StandingTone,
  TodayPanel,
  WrittenBy,
} from "./record360";
import "./dealstatus.css";

// Deal360 — the deal page's written briefing, read before a call, in the
// cards every record page reads in.
//
// THE CALL: the standing the server reached, the first line of its reasoning
// beside it, the findings that tripped, and the deal's own thread under them.
// THE DAY'S WORK: the one move the briefing names, with the verb that performs
// it. THE BRIEF: what has happened and what the buyer wants, in prose with its
// sources, and the rest of the reasoning behind a fold for the reader who
// wants it. Everything else on the page is a record; this is the reading.
//
// The READ writes nothing a user can see: the server caches the briefing per
// reader, so a repeat view costs nothing and a changed deal is rewritten
// before the read answers. The CLICK performs the move through the verb the
// briefing names — the task door, the compose modal, the meeting-brief drawer
// — so nothing here re-implements a write or skips its gates.

type DealStatusCard = components["schemas"]["DealStatusCard"];
type DealStatusCardMove = components["schemas"]["DealStatusCardMove"];
type DealStatusCardSection = components["schemas"]["DealStatusCardSection"];

// The verdict words the server may send, and how each reads to a person. A
// word this build does not know renders as itself rather than as nothing:
// the reader has learned four, and a fifth arriving from a newer server is
// still a call the card must show.
const VERDICT_LABELS: Record<string, MessageKey> = {
  live: "deal360.verdict.live",
  drifting: "deal360.verdict.drifting",
  blocked: "deal360.verdict.blocked",
  cold: "deal360.verdict.cold",
};

// How loud each standing is. `live` is untoned deliberately: a card that
// colours every state has no colour left for the one that needs it, and a
// healthy deal shouting is how a reader learns to stop looking at the strip.
const VERDICT_TONE: Record<string, StandingTone> = {
  live: "calm",
  drifting: "warn",
  blocked: "danger",
  cold: "danger",
};

// The tone for a standing this build does not know. Own-property lookup, and
// NOT the healthy tone: a fifth word from a newer server is a call the card
// must show, and colouring it green would report an unreadable word as good
// news in the loudest element on the page.
function verdictTone(standing: string): StandingTone {
  return Object.hasOwn(VERDICT_TONE, standing)
    ? VERDICT_TONE[standing]
    : "unknown";
}

// The card's read, shared. Deal360 draws the briefing from it and the email
// box takes `reply_to` out of the same entry, so the two cannot end up
// disagreeing about whether there is a message waiting for an answer — and the
// box costs no second request.
export function useDealStatusCard(dealId: string) {
  const t = useT();
  return useQuery({
    queryKey: ["deal-status", dealId],
    queryFn: async () => {
      const { data, error } = await api.GET("/deals/{id}/status", {
        params: { path: { id: dealId } },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
  });
}

export function DealStatusCardPanel({
  dealId,
  dealName,
  pulse,
  spine,
}: Readonly<{
  dealId: string;
  dealName: string;
  // Whose move it is, in one sentence (DealPulse). Under the call rather than
  // in the header: it is a reading of the same status card, and the sentence
  // the standing rests on.
  pulse?: ReactNode;
  // The deal's story as a thread, under the call it was read from. Handed in
  // because the rows it is drawn from are the page's timeline, which this
  // card does not read.
  spine?: ReactNode;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const status = useDealStatusCard(dealId);
  const rewrite = useMutation({
    mutationKey: ["deal-status-refresh", dealId],
    mutationFn: async () => {
      const { data, error } = await api.GET("/deals/{id}/status", {
        params: { path: { id: dealId }, query: { refresh: true } },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: (data) =>
      queryClient.setQueryData(["deal-status", dealId], data),
  });
  if (status.data?.story) {
    return (
      <Briefing
        dealId={dealId}
        dealName={dealName}
        card={status.data}
        pulse={pulse}
        spine={spine}
        onRewrite={() => rewrite.mutate()}
        rewriting={rewrite.isPending}
      />
    );
  }
  // Neither the pending read nor the failed one draws the call. That card
  // exists to state a verdict, and a card holding a spinner where the verdict
  // goes is the reading claiming to have reached one. The head every written
  // reading carries still names what is being read.
  return (
    <>
      <CallCard name={dealName}>
        <QueryStates query={status} pendingLines={6}>
          {status.isSuccess && !status.data?.story ? (
            // `story` is required on the wire and the server always writes at
            // least one line, so reaching here means a response that is not
            // the shape the contract promises. Saying so beats an empty panel,
            // which reads as a deal nobody has touched.
            <PanelBody>
              <p className="t-small">{t("deal360.unreadable")}</p>
            </PanelBody>
          ) : null}
        </QueryStates>
      </CallCard>
      {/* The day's work keeps its place under the call in every state of the
          read: the page has one shape, and a panel that comes and goes with a
          request reads as a record that has and then has not any work. What
          it says follows the read — pending, failed — rather than claiming a
          quiet day it has not established. */}
      <TodayPanel
        state={status.isPending ? "loading" : "failed"}
        onOpenTasks={() => navigate({ screen: "worklist" })}
      />
    </>
  );
}

function Briefing({
  dealId,
  dealName,
  card,
  pulse,
  spine,
  onRewrite,
  rewriting,
}: Readonly<{
  dealId: string;
  dealName: string;
  card: DealStatusCard;
  pulse?: ReactNode;
  spine?: ReactNode;
  onRewrite: () => void;
  rewriting: boolean;
}>) {
  const t = useT();
  const open = (entityType: string, entityId: string) => {
    if (entityType === "deal") {
      navigate({ screen: "deals", id: entityId });
    } else if (entityType === "person") {
      navigate({ screen: "contacts", id: entityId });
    }
  };
  // The findings ride the coverage card's own query, so this costs no second
  // request and the two cannot disagree about what is wrong with the deal.
  const coverage = useDealSignals(dealId, true);
  const because = card.verdict?.because.sentences ?? [];
  return (
    <>
      {/* The call, first and alone. It used to sit fourth, under three
          paragraphs — which meant the one word a reader scanning thirty deals
          needs was the last thing they reached. The lead sentence keeps its
          RECEIPTS: it is the sentence a reader is most likely to challenge, on
          a card whose whole premise is that they can. */}
      <CallCard
        name={dealName}
        standing={
          card.verdict
            ? {
                label: verdictLabel(card.verdict.standing, t),
                tone: verdictTone(card.verdict.standing),
              }
            : undefined
        }
        because={
          because.length > 0 ? (
            <SentenceList sentences={because.slice(0, 1)} onOpenRecord={open} />
          ) : undefined
        }
      >
        {pulse ? <PanelBody>{pulse}</PanelBody> : null}
        <SignalStrip signals={coverage.signals} />
        {spine}
      </CallCard>
      <TodayPanel onOpenTasks={() => navigate({ screen: "worklist" })}>
        {card.next ? (
          <Move key="next" dealId={dealId} move={card.next} />
        ) : null}
      </TodayPanel>
      {/* The reading, under the call and the work: what has happened and
          where that leaves things, in prose with its sources. What is holding
          the deal up, what the buyer wants and the rest of the reasoning sit
          behind a fold — every word still here and still cited, and none of it
          between a scanning reader and the call. */}
      <Panel
        title={t("deal360.brief")}
        titleAction={<WrittenBy by={card.generated_by} />}
        footer={
          <div className="deal360-foot">
            <Button
              variant="ghost"
              small
              pending={rewriting}
              onClick={onRewrite}
            >
              <RefreshCw aria-hidden />
              {t("deal360.rewrite")}
            </Button>
          </div>
        }
      >
        <Section section={card.story} onOpenRecord={open} lead />
        <details className="deal360-fold">
          <summary>{t("deal360.readFull")}</summary>
          <Section
            heading={t("deal360.blocker")}
            section={card.blocker}
            onOpenRecord={open}
            tone="warn"
          />
          <Section
            heading={t("deal360.buyer")}
            section={card.buyer}
            onOpenRecord={open}
          />
          {/* The rest of the verdict's reasoning. Its first line is already in
              the call above, so this renders only what the head did not. */}
          {because.length > 1 ? (
            <Section
              section={{ sentences: because.slice(1) }}
              onOpenRecord={open}
            />
          ) : null}
        </details>
      </Panel>
    </>
  );
}

// Section renders one headed block, and renders NOTHING when the server sent
// none. An absent section means the records did not support saying anything —
// an empty "what is holding this up" would read as "nothing is".
function Section({
  heading,
  section,
  onOpenRecord,
  tone,
  lead,
}: Readonly<{
  heading?: string;
  section: DealStatusCardSection | undefined;
  onOpenRecord: (entityType: string, entityId: string) => void;
  tone?: "warn";
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
        <p className={tone === "warn" ? "t-caption deal360-warn" : "t-caption"}>
          {heading}
        </p>
      ) : null}
      <SentenceList
        sentences={section.sentences}
        onOpenRecord={onOpenRecord}
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

// activityIdOf reads the one operand the meeting-brief verb takes. The
// arguments object is typed open on the wire; a missing id renders no button
// rather than a button that would 404.
function activityIdOf(move: DealStatusCardMove): string | null {
  const raw = move.arguments?.activity_id;
  return typeof raw === "string" && raw !== "" ? raw : null;
}

// Whether the verb has what it needs to be drawn, decided BEFORE the slot is:
// a slot handed a control that renders nothing still draws the slot, and an
// empty action region reads as a verb that failed to load. MoveButton keeps
// its own null returns for the narrowing the cases need; this is what decides
// whether the row has a verb at all.
function hasMoveControl(move: DealStatusCardMove): boolean {
  switch (move.action) {
    case "create_task":
      return Boolean(move.arguments);
    case "open_meeting_brief":
      return activityIdOf(move) !== null;
    default:
      return false;
  }
}

// The move the briefing names, as the one row the agent is asking for: the
// reason is the ask, the evidence under it is what it rests on, and the verb
// at the row's end performs it.
function Move({
  dealId,
  move,
}: Readonly<{ dealId: string; move: DealStatusCardMove }>) {
  return (
    <FoundMove
      title={move.reason}
      basis={
        move.evidence.length > 0 ? (
          <ul className="deal360-evidence t-small">
            {move.evidence.map((row) => (
              <li key={`${row.activity_id ?? ""}-${row.text}`}>{row.text}</li>
            ))}
          </ul>
        ) : undefined
      }
      action={
        hasMoveControl(move) ? (
          <MoveButton dealId={dealId} move={move} />
        ) : undefined
      }
    />
  );
}

function MoveButton({
  dealId,
  move,
}: Readonly<{ dealId: string; move: DealStatusCardMove }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [briefOpen, setBriefOpen] = useState(false);
  const activityId = activityIdOf(move);
  const createTask = useMutation({
    mutationKey: ["deal-status-create-task"],
    // The arguments ARE the task body the server prepared; the click sends
    // them as they came rather than re-deriving them from render state.
    mutationFn: async (body: Record<string, unknown>) => {
      const { data, error } = await api.POST("/tasks", {
        body: body as components["schemas"]["CreateTaskRequest"],
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["deal-status", dealId] });
      queryClient.invalidateQueries({ queryKey: ["activities"] });
      queryClient.invalidateQueries({ queryKey: ["tasks"] });
    },
  });

  const taskBody = move.arguments;
  switch (move.action) {
    case "create_task":
      // No body, no button: a click that sent {} would only be refused.
      if (!taskBody) {
        return null;
      }
      return (
        <>
          <Button
            variant="primary"
            small
            pending={createTask.isPending}
            onClick={() => createTask.mutate(taskBody)}
          >
            <ListChecks aria-hidden />
            {t("deal360.createTask")}
          </Button>
          {createTask.isError ? (
            <p className="t-small t-danger">
              {problemMessageOf(createTask.error, t)}
            </p>
          ) : null}
        </>
      );
    case "draft_email":
      // No button here. Writing to the buyer is the email box's job, in the
      // right-hand column under the Deal Room — one place a rep goes to send
      // mail, whether or not this move happens to rank first. Deal360 still
      // says WHY the mail is the move; it just does not carry a second door
      // to the same composer.
      return null;
    case "open_meeting_brief":
      if (!activityId) {
        return null;
      }
      return (
        <>
          <Button variant="primary" small onClick={() => setBriefOpen(true)}>
            <Sparkles aria-hidden />
            {t("deal360.openBrief")}
          </Button>
          <PersonMeetingBrief
            activityId={activityId}
            open={briefOpen}
            onClose={() => setBriefOpen(false)}
          />
        </>
      );
    default:
      return null;
  }
}
