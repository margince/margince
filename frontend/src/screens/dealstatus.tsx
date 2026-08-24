import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ListChecks, RefreshCw, Sparkles } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import { PersonMeetingBrief } from "./persondrawers";
import { SentenceList, WrittenBy } from "./record360";
import "./dealstatus.css";

// Deal360 — the deal page's written briefing, read before a call.
//
// It leads the page rather than sitting in the margin, because it is the one
// thing on the screen that answers "what do I need to know" instead of "what
// is recorded". Everything else on the page is a record; this is the reading.
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
// word this build does not know renders as no badge rather than as itself:
// the reader has learned four, and a fifth in raw form teaches them nothing.
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

export function DealStatusCardPanel({ dealId }: Readonly<{ dealId: string }>) {
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
  return (
    <Panel title={t("deal360.title")} sub={t("deal360.sub")} tone="accent">
      <QueryStates query={status} pendingLines={6}>
        {status.data?.story ? (
          <Briefing
            dealId={dealId}
            card={status.data}
            onRewrite={() => rewrite.mutate()}
            rewriting={rewrite.isPending}
          />
        ) : null}
        {status.isSuccess && !status.data?.story ? (
          // `story` is required on the wire and the server always writes at
          // least one line, so reaching here means a response that is not the
          // shape the contract promises. Saying so beats an empty panel, which
          // reads as a deal nobody has touched.
          <PanelBody>
            <p className="t-small">{t("deal360.unreadable")}</p>
          </PanelBody>
        ) : null}
      </QueryStates>
    </Panel>
  );
}

function Briefing({
  dealId,
  card,
  onRewrite,
  rewriting,
}: Readonly<{
  dealId: string;
  card: DealStatusCard;
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
  return (
    <>
      <Section section={card.story} onOpenRecord={open} />
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
      {card.verdict ? <Verdict verdict={card.verdict} onOpen={open} /> : null}
      {card.next ? <Move dealId={dealId} move={card.next} /> : null}
      <PanelBody>
        <div className="deal360-foot">
          <WrittenBy by={card.generated_by} />
          <Button variant="ghost" pending={rewriting} onClick={onRewrite}>
            <RefreshCw aria-hidden />
            {t("deal360.rewrite")}
          </Button>
        </div>
      </PanelBody>
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
}: Readonly<{
  heading?: string;
  section: DealStatusCardSection | undefined;
  onOpenRecord: (entityType: string, entityId: string) => void;
  tone?: "warn";
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
      <SentenceList sentences={section.sentences} onOpenRecord={onOpenRecord} />
    </PanelBody>
  );
}

function Verdict({
  verdict,
  onOpen,
}: Readonly<{
  verdict: NonNullable<DealStatusCard["verdict"]>;
  onOpen: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const label = VERDICT_LABELS[verdict.standing];
  return (
    <PanelBody>
      <div className="deal360-verdict-head">
        <p className="t-caption">{t("deal360.verdict")}</p>
        {label ? (
          <span className={`deal360-standing deal360-${verdict.standing}`}>
            {t(label)}
          </span>
        ) : null}
      </div>
      <SentenceList
        sentences={verdict.because.sentences}
        onOpenRecord={onOpen}
      />
    </PanelBody>
  );
}

// activityIdOf reads the one operand the meeting-brief verb takes. The
// arguments object is typed open on the wire; a missing id renders no button
// rather than a button that would 404.
function activityIdOf(move: DealStatusCardMove): string | null {
  const raw = move.arguments?.activity_id;
  return typeof raw === "string" && raw !== "" ? raw : null;
}

function Move({
  dealId,
  move,
}: Readonly<{ dealId: string; move: DealStatusCardMove }>) {
  const t = useT();
  return (
    <PanelBody>
      <p className="t-caption">{t("deal360.next")}</p>
      <p className="deal360-move-reason">{move.reason}</p>
      <MoveButton dealId={dealId} move={move} />
      {move.evidence.length > 0 ? (
        <ul className="deal360-evidence t-small">
          {move.evidence.map((row) => (
            <li key={`${row.activity_id ?? ""}-${row.text}`}>{row.text}</li>
          ))}
        </ul>
      ) : null}
    </PanelBody>
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
          <Button variant="primary" onClick={() => setBriefOpen(true)}>
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
