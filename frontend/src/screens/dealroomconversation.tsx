import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import {
  AddDocument,
  DOCUMENT_GROUPS,
  useRemoveDocument,
  useRoomDocuments,
} from "./dealroomdocuments";
import { type BoardDocument, DocumentBoard } from "./dealroomthreads";

// The seller's side of the document board: the documents with their share
// state, the threads under each, and the verbs the seller has — add, remove,
// reply, open, resolve.

type DealRoom = components["schemas"]["DealRoom"];
type DealRoomDocument = components["schemas"]["DealRoomDocument"];

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
  const documents: BoardDocument[] = (docs.data?.data ?? []).map((doc) => ({
    id: doc.id,
    groupKey: doc.group_key,
    title: doc.title,
    meta: doc.filename && doc.filename !== doc.title ? doc.filename : "",
    actions: <RemoveButton room={room} doc={doc} refusal={refusal} />,
  }));
  return (
    <QueryStates
      query={docs}
      pendingLines={3}
      pendingLabel={t("room.docs.title")}
    >
      <QueryStates
        query={threads}
        pendingLines={3}
        pendingLabel={t("room.docs.title")}
      >
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
    </QueryStates>
  );
}
