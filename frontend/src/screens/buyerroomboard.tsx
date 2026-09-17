// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download } from "lucide-react";
import { useEffect } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { previewMediaType } from "../design-system/filechip";
import { useFilePreview } from "../design-system/filepreview";
import { useT } from "../i18n";
import {
  bearer,
  refuseOrThrow,
  retireOnRefusal,
  SessionRefusedError,
} from "./buyerroomsession";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import { DOCUMENT_GROUPS } from "./dealroomdocuments";
import { type BoardDocument, DocumentBoard } from "./dealroomthreads";
import { downloadBytes } from "./download";

// The buyer's half of the document board: the shared documents and the
// conversation, read and written on the room session's Bearer. The board
// itself is drawn once for both sides in dealroomthreads.tsx; this file is
// what the buyer brings to it — download, read in place, ask and reply — and
// never resolve.

// The buyer's documents query, shared by the board and the decision verbs so
// one request serves both.
function useBuyerDocuments(token: string, onSessionLost: () => void) {
  const t = useT();
  const docs = useQuery({
    queryKey: ["buyer-room-documents", token],
    retry: false,
    // Re-asked with the tab, for the same reason /public/rooms/me is: a release
    // published while the buyer was away is what they came back to read.
    refetchOnWindowFocus: "always",
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/public/rooms/documents",
        { ...bearer(token) },
      );
      if (error) {
        if (response.status === 401) {
          throw new SessionRefusedError();
        }
        throwProblem(error, t);
      }
      return data;
    },
  });
  const lost = docs.error instanceof SessionRefusedError;
  useEffect(() => {
    if (lost) {
      onSessionLost();
    }
  }, [lost, onSessionLost]);
  return docs;
}

type BuyerRoomDocument = components["schemas"]["BuyerRoomDocument"];

// The buyer's one verb on a document: take a copy of it.
//
// No "confirm this version" and no "request changes". Asking a buyer to
// formally accept each file turns a room into an approval queue nobody asked
// for, and one reading "Confirm this version" under a transcript cannot tell
// what they would be agreeing to. What they want to say goes in the thread
// under it, which is the whole point of the board.
function BuyerDocumentVerbs({
  token,
  doc,
}: Readonly<{ token: string; doc: BuyerRoomDocument }>) {
  const t = useT();
  const download = useMutation({
    mutationKey: ["buyer-room-document-download"],
    // The failure line rides as a mutation VARIABLE rather than off `t` inside
    // the function: a mutationFn is re-armed in a passive effect, so a closure
    // read here is the render-before-last's — the locale just left behind.
    mutationFn: async (input: {
      documentId: string;
      filename: string;
      failure: string;
      token: string;
    }) => {
      const { data, error, response } = await api.GET(
        "/public/rooms/documents/{documentId}/file",
        {
          params: { path: { documentId: input.documentId } },
          parseAs: "blob",
          ...bearer(input.token),
        },
      );
      if (error || !data) {
        // A refusal this screen decided, with copy it already translated, so
        // it rides as a problem body — a plain Error is wording nobody wrote
        // for a user and is replaced by the shared failure line.
        throwProblem({ status: response.status, detail: input.failure });
      }
      // The blob's OWN type, because the server chose it: a PDF handed to the
      // reader as application/octet-stream downloads with the wrong icon and
      // opens in nothing.
      downloadBytes(data, input.filename, data.type);
    },
  });
  return (
    <div className="buyer-doc-actions">
      <Button
        aria-label={t("buyer.docs.download", { title: doc.title })}
        pending={download.isPending}
        onClick={() =>
          download.mutate({
            documentId: doc.id,
            filename: doc.filename,
            failure: t("buyer.docs.downloadFailed"),
            token,
          })
        }
      >
        <Download aria-hidden />
        {t("buyer.docs.downloadShort")}
      </Button>
      {download.isError ? (
        <p className="t-danger">{problemMessageOf(download.error, t)}</p>
      ) : null}
    </div>
  );
}

// The buyer's board: the shared documents, each with the threads about it,
// and the room-wide conversation. The verbs are the buyer's — download, open,
// reply — and never resolve.
export function BuyerBoard({
  token,
  onSessionLost,
  mayWrite,
  refusal,
}: Readonly<{
  token: string;
  onSessionLost: () => void;
  mayWrite: boolean;
  refusal: string | undefined;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const preview = useFilePreview();
  const docs = useBuyerDocuments(token, onSessionLost);
  const threads = useQuery({
    queryKey: ["buyer-room-threads", token],
    retry: false,
    // The conversation is live on both sides, so a returning tab re-reads it.
    refetchOnWindowFocus: "always",
    queryFn: async () => {
      const { data, error, response } = await api.GET("/public/rooms/threads", {
        ...bearer(token),
      });
      if (error) {
        if (response.status === 401) {
          throw new SessionRefusedError();
        }
        throwProblem(error, t);
      }
      return data;
    },
  });
  const lost = threads.error instanceof SessionRefusedError;
  useEffect(() => {
    if (lost) {
      onSessionLost();
    }
  }, [lost, onSessionLost]);
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ["buyer-room-threads", token] });
  const open = useMutation({
    mutationKey: ["buyer-room-thread-open"],
    mutationFn: async (input: {
      documentId: string | null;
      body: string;
      requiredChange: boolean;
    }) => {
      const { data, error, response } = await api.POST(
        "/public/rooms/threads",
        {
          body: {
            document_id: input.documentId,
            body: input.body,
            required_change: input.requiredChange,
          },
          ...bearer(token),
        },
      );
      if (error) {
        refuseOrThrow(error, response, t);
      }
      return data;
    },
    onError: retireOnRefusal(onSessionLost),
    onSuccess: refresh,
  });
  const reply = useMutation({
    mutationKey: ["buyer-room-thread-reply"],
    mutationFn: async (input: { threadId: string; body: string }) => {
      const { data, error, response } = await api.POST(
        "/public/rooms/threads/{threadId}/comments",
        {
          params: { path: { threadId: input.threadId } },
          body: { body: input.body },
          ...bearer(token),
        },
      );
      if (error) {
        refuseOrThrow(error, response, t);
      }
      return data;
    },
    onError: retireOnRefusal(onSessionLost),
    onSuccess: refresh,
  });
  const documents: BoardDocument[] = (docs.data?.data ?? []).map((doc) => {
    // Read here, over the room, rather than in the downloads folder: the bytes
    // ride the same Bearer the list did, and the preview re-types them from
    // the filename exactly as it does for a seat's own files. A kind no
    // browser draws keeps the download as its only verb.
    const mediaType = previewMediaType(doc.filename);
    const read =
      preview !== null && mediaType !== null
        ? () =>
            preview.open({
              href: `/v1/public/rooms/documents/${doc.id}/file`,
              filename: doc.filename,
              mediaType,
              bearer: token,
            })
        : undefined;
    return {
      id: doc.id,
      groupKey: doc.group_key,
      title: doc.title,
      filename: doc.filename,
      meta: doc.filename,
      byteSize: doc.byte_size,
      read,
      actions: <BuyerDocumentVerbs token={token} doc={doc} />,
    };
  });
  return (
    <QueryStates
      query={docs}
      pendingLines={3}
      pendingLabel={t("buyer.docs.title")}
    >
      <QueryStates
        query={threads}
        pendingLines={3}
        pendingLabel={t("buyer.docs.title")}
      >
        {docs.data && threads.data ? (
          <DocumentBoard
            title={t("buyer.docs.title")}
            groups={DOCUMENT_GROUPS.map((g) => ({
              key: g.key,
              label: t(g.labelKey),
            }))}
            documents={documents}
            threads={threads.data.data}
            empty={t("buyer.docs.empty")}
            verbs={{
              mayRequireChange: true,
              refusal,
              open: mayWrite ? (input) => open.mutateAsync(input) : undefined,
              reply: mayWrite
                ? (threadId, body) => reply.mutateAsync({ threadId, body })
                : undefined,
            }}
          />
        ) : null}
      </QueryStates>
    </QueryStates>
  );
}
