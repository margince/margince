import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import { useRoomChanges } from "./dealroomchanges";
import {
  AddDocument,
  DOCUMENT_GROUPS,
  useRemoveDocument,
  useRoomDocuments,
} from "./dealroomdocuments";
import { type BoardDocument, DocumentBoard } from "./dealroomthreads";

// The seller's side of the document board: the documents with their share
// state, the threads under each, the buyer's decisions, and the verbs the
// seller has — add, remove, reply, open, resolve.

type DealRoom = components["schemas"]["DealRoom"];
type DealRoomDocument = components["schemas"]["DealRoomDocument"];
type DealRoomChange = components["schemas"]["DealRoomChange"];
type DealRoomDecision = components["schemas"]["DealRoomDecision"];

// What the buyer has of this document, read off the unpublished change list:
// a document the next release would add is not yet with the buyer; one it
// would retitle or regroup is with them in an older form; a document the
// list does not name is with them as it is. A room never published has
// shared nothing.
function shareState(
  doc: DealRoomDocument,
  room: DealRoom,
  changes: readonly DealRoomChange[] | undefined,
): "unshared" | "changed" | "shared" | "unknown" {
  if (!changes) {
    return "unknown";
  }
  if (!room.published_at) {
    return "unshared";
  }
  const mine = changes.filter((c) => c.document_id === doc.id);
  if (mine.some((c) => c.kind === "document_added")) {
    return "unshared";
  }
  return mine.length > 0 ? "changed" : "shared";
}

function ShareBadge({
  state,
}: Readonly<{ state: ReturnType<typeof shareState> }>) {
  const t = useT();
  switch (state) {
    case "unshared":
      return <Badge tone="warn">{t("room.docs.unshared")}</Badge>;
    case "changed":
      return <Badge tone="warn">{t("room.docs.changed")}</Badge>;
    case "shared":
      return <Badge tone="success">{t("room.docs.shared")}</Badge>;
    default:
      return null;
  }
}

function RemoveButton({
  room,
  doc,
  refusal,
}: Readonly<{ room: DealRoom; doc: DealRoomDocument; refusal?: string }>) {
  const t = useT();
  const remove = useRemoveDocument(room.id);
  return (
    <>
      <Button
        small
        iconOnly
        aria-label={t("room.docs.remove", { title: doc.title })}
        reason={refusal}
        pending={remove.isPending}
        onClick={() =>
          remove.mutate({ documentId: doc.id, version: doc.version })
        }
      >
        <Trash2 aria-hidden />
      </Button>
      {remove.isError ? (
        <p className="t-small t-danger">{problemMessageOf(remove.error, t)}</p>
      ) : null}
    </>
  );
}

// The latest decision a reviewer made on this document, as one sentence.
function DecisionNote({
  decisions,
}: Readonly<{ decisions: readonly DealRoomDecision[] }>) {
  const t = useT();
  if (decisions.length === 0) {
    return null;
  }
  const latest = decisions.reduce((a, b) =>
    b.created_at > a.created_at ? b : a,
  );
  return (
    <p className="t-small">
      <strong>{latest.participant_name}</strong>{" "}
      {t(
        latest.kind === "confirm_version"
          ? "room.decisions.confirm_version"
          : "room.decisions.request_changes",
      )}
      {latest.note ? ` — ${latest.note}` : ""}
    </p>
  );
}

export function DealRoomConversation({
  room,
  refusal,
}: Readonly<{ room: DealRoom; refusal: string | undefined }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const threads = useQuery({
    queryKey: ["deal-room-threads", room.id],
    queryFn: async () => {
      const { data, error } = await api.GET("/deal-rooms/{id}/threads", {
        params: { path: { id: room.id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const docs = useRoomDocuments(room.id);
  const changes = useRoomChanges(room.id);
  const decisions = useRoomDecisions(room.id);
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ["deal-room-threads", room.id] });
  const open = useMutation({
    mutationKey: ["deal-room-thread-open"],
    mutationFn: async (input: {
      documentId: string | null;
      body: string;
      requiredChange: boolean;
    }) => {
      const { data, error } = await api.POST("/deal-rooms/{id}/threads", {
        params: { path: { id: room.id } },
        body: {
          document_id: input.documentId,
          body: input.body,
          required_change: input.requiredChange,
          source: "ui",
        },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: refresh,
  });
  const reply = useMutation({
    mutationKey: ["deal-room-thread-reply"],
    mutationFn: async (input: { threadId: string; body: string }) => {
      const { data, error } = await api.POST(
        "/deal-rooms/{id}/threads/{threadId}/comments",
        {
          params: { path: { id: room.id, threadId: input.threadId } },
          body: { body: input.body, source: "ui" },
        },
      );
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: refresh,
  });
  const resolve = useMutation({
    mutationKey: ["deal-room-thread-resolve"],
    mutationFn: async (threadId: string) => {
      const { data, error } = await api.POST(
        "/deal-rooms/{id}/threads/{threadId}/resolve",
        { params: { path: { id: room.id, threadId } } },
      );
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: refresh,
  });
  const mayWrite = refusal === undefined;
  const decided = decisions.data?.data ?? [];
  const documents: BoardDocument[] = (docs.data?.data ?? []).map((doc) => ({
    id: doc.id,
    groupKey: doc.group_key,
    title: doc.title,
    meta: doc.filename && doc.filename !== doc.title ? doc.filename : "",
    status: <ShareBadge state={shareState(doc, room, changes.data?.changes)} />,
    actions: <RemoveButton room={room} doc={doc} refusal={refusal} />,
    note: (
      <DecisionNote
        decisions={decided.filter((d) => d.document_id === doc.id)}
      />
    ),
  }));
  return (
    <QueryStates query={threads} pendingLines={3}>
      {threads.data && docs.data ? (
        <DocumentBoard
          title={t("room.docs.title")}
          sub={t("room.docs.sub")}
          groups={DOCUMENT_GROUPS.map((g) => ({
            key: g.key,
            label: t(g.labelKey),
          }))}
          documents={documents}
          threads={threads.data.data}
          empty={t("room.docs.empty")}
          footer={<AddDocument room={room} refusal={refusal} />}
          verbs={{
            mayRequireChange: false,
            refusal,
            open: mayWrite ? (input) => open.mutateAsync(input) : undefined,
            reply: mayWrite
              ? (threadId, body) => reply.mutateAsync({ threadId, body })
              : undefined,
            resolve: mayWrite
              ? (threadId) => resolve.mutateAsync(threadId)
              : undefined,
          }}
        />
      ) : null}
    </QueryStates>
  );
}

function useRoomDecisions(roomId: string) {
  return useQuery({
    queryKey: ["deal-room-decisions", roomId],
    queryFn: async () => {
      const { data, error } = await api.GET("/deal-rooms/{id}/decisions", {
        params: { path: { id: roomId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
