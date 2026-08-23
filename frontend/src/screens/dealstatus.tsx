import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  ListChecks,
  Mail,
  RefreshCw,
  Sparkles,
} from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import { SentenceList, WrittenBy } from "./company360";
import { ComposeModal } from "./compose";
import { PersonMeetingBrief } from "./persondrawers";
import "./dealstatus.css";

// The deal page's one card: where the deal stands, what could lose it, and the
// one move to make. It replaces the three that stood here before — a brief, a
// health score and a next-move box — which asked the reader to decide which to
// believe.
//
// The READ writes nothing a user can see: the server caches the card per
// reader, so a repeat view costs nothing and a changed deal is rewritten
// before the read answers. The CLICK performs the move through the verb the
// card names — the task door, the compose modal, the meeting-brief drawer —
// so nothing here re-implements a write or skips its gates.

type DealStatusCard = components["schemas"]["DealStatusCard"];
type DealStatusCardMove = components["schemas"]["DealStatusCardMove"];

export function DealStatusCardPanel({ dealId }: Readonly<{ dealId: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const status = useQuery({
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
    <Panel
      title={t("dealstatus.title")}
      sub={t("dealstatus.sub")}
      tone="accent"
    >
      <QueryStates query={status} pendingLines={4}>
        {status.data?.standing ? (
          <StatusBody
            dealId={dealId}
            card={status.data}
            onRewrite={() => rewrite.mutate()}
            rewriting={rewrite.isPending}
          />
        ) : null}
        {status.isSuccess && !status.data?.standing ? (
          // `standing` is required on the wire and the server always writes at
          // least one line, so reaching here means a response that is not the
          // shape the contract promises. Saying so beats an empty panel with a
          // title, which reads as a deal nobody has touched.
          <PanelBody>
            <p className="t-small">{t("dealstatus.unreadable")}</p>
          </PanelBody>
        ) : null}
      </QueryStates>
    </Panel>
  );
}

function StatusBody({
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
      <PanelBody>
        <SentenceList sentences={card.standing.sentences} onOpenRecord={open} />
      </PanelBody>
      {card.risk ? (
        <PanelBody>
          <p className="t-caption dealstatus-risk-head">
            <AlertTriangle aria-hidden />
            {t("dealstatus.risk")}
          </p>
          <SentenceList sentences={card.risk.sentences} onOpenRecord={open} />
        </PanelBody>
      ) : null}
      {card.next ? (
        <PanelBody>
          <div className="dealstatus-move">
            <p className="dealstatus-move-reason">{card.next.reason}</p>
            <MoveButton dealId={dealId} move={card.next} />
            {(card.next.evidence ?? []).length > 0 ? (
              <ul className="dealstatus-evidence t-small">
                {(card.next.evidence ?? []).map((row) => (
                  <li key={`${row.activity_id ?? ""}-${row.text}`}>
                    {row.text}
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        </PanelBody>
      ) : null}
      <PanelBody>
        <div className="dealstatus-foot">
          <WrittenBy by={card.generated_by} />
          <Button variant="ghost" pending={rewriting} onClick={onRewrite}>
            <RefreshCw aria-hidden />
            {t("dealstatus.rewrite")}
          </Button>
        </div>
      </PanelBody>
    </>
  );
}

// activityIdOf reads the one operand the navigation and reply verbs take. The
// arguments object is typed open on the wire; a missing id renders no button
// rather than a button that would 404.
function activityIdOf(move: DealStatusCardMove): string | null {
  const raw = move.arguments?.activity_id;
  return typeof raw === "string" && raw !== "" ? raw : null;
}

function MoveButton({
  dealId,
  move,
}: Readonly<{ dealId: string; move: DealStatusCardMove }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [composing, setComposing] = useState(false);
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
            {t("dealstatus.createTask")}
          </Button>
          {createTask.isError ? (
            <p className="t-small t-danger">
              {problemMessageOf(createTask.error, t)}
            </p>
          ) : null}
        </>
      );
    case "draft_email":
      if (!activityId) {
        return null;
      }
      return (
        <>
          <Button variant="primary" onClick={() => setComposing(true)}>
            <Mail aria-hidden />
            {t("dealstatus.draftReply")}
          </Button>
          {composing ? (
            <ComposeModal
              activityId={activityId}
              entityType="deal"
              entityId={dealId}
              kind="email"
              open={composing}
              onClose={() => setComposing(false)}
            />
          ) : null}
        </>
      );
    case "open_meeting_brief":
      if (!activityId) {
        return null;
      }
      return (
        <>
          <Button variant="primary" onClick={() => setBriefOpen(true)}>
            <Sparkles aria-hidden />
            {t("dealstatus.openBrief")}
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
