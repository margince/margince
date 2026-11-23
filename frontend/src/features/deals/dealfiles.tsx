import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { AddDocumentDialog } from "../../components/adddocument";
import {
  AttachmentGrid,
  AttachmentTile,
} from "../../components/attachmenttile";
import { ChatCapture } from "../../components/chatcapture";
import { FileVerbs } from "../../components/filerowverbs";
import { Badge, Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { undoAction, useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { dealFilesKey, dealHideKeys } from "../../lib/attachmentkeys";
import { useCanWriteRecord } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";

// The deal's Files area: what a rep uploaded on the deal, and what arrived
// with the messages linked to it. The second half is why this exists — an
// emailed contract lives on the email, and before this read it was reachable
// from the timeline alone.
//
// Two kinds of row, two verbs. A file uploaded HERE belongs to the deal and
// can be deleted. A captured file belongs to its message: the deal can only
// stop listing it (hide), and the file stays on the activity and in the
// company library. So a hide runs at once with an Undo, and a delete asks
// first.

type Deal = components["schemas"]["Deal"];
type DealDocument = components["schemas"]["DealDocument"];

// Enough for a working set; the area is not a library somebody pages through.
const PAGE_LIMIT = 100;

export function dealDocumentsKey(dealId: string, includeHidden: boolean) {
  return [...dealFilesKey(dealId), includeHidden] as const;
}

// The deal's files, one read for the tab and for the count on its label, so
// the two never disagree about how many there are.
export function useDealDocuments(dealId: string, includeHidden = false) {
  return useQuery({
    queryKey: dealDocumentsKey(dealId, includeHidden),
    queryFn: async () => {
      return unwrap(
        await api.GET("/deals/{id}/documents", {
          params: {
            path: { id: dealId },
            query: { limit: PAGE_LIMIT, include_hidden: includeHidden },
          },
        }),
      );
    },
  });
}

export function DealFiles({
  deal,
  onOpenEmail,
}: Readonly<{
  deal: Deal;
  /** Opens a mail in the page's drawer, from the line naming a file's mail. */
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const dealId = deal.id;
  // useCanWriteRecord, not useCanWrite: every write here — the upload, a
  // removal, hiding a captured file — runs through the DEAL's own write gate
  // on the server, so the object grant alone offered Add file on a colleague's
  // deal and refused the upload once the bytes were chosen.
  const mayWrite = useCanWriteRecord("deal", deal);
  const [adding, setAdding] = useState(false);
  const [capturing, setCapturing] = useState(false);
  const [showHidden, setShowHidden] = useState(false);
  const listRegion = useRef<HTMLDivElement | null>(null);
  const focusLanding = useCallback(() => listRegion.current, []);
  const query = useDealDocuments(dealId, showHidden);
  const files = query.data?.data ?? [];

  let state: SectionState;
  if (query.isPending) {
    state = "loading";
  } else if (files.length === 0) {
    state = query.isError ? "failed" : "empty";
  } else {
    state = "ready";
  }

  return (
    <>
      {capturing && (
        <ChatCapture
          record={{ type: "deal", id: dealId, name: deal.name }}
          onClose={() => setCapturing(false)}
        />
      )}
      <Panel
        title={t("files.title")}
        titleAction={
          mayWrite ? (
            <>
              <Button
                variant="ai"
                disabled={capturing}
                onClick={() => setCapturing(true)}
              >
                {t("chatCapture.action")}
              </Button>
              <Button onClick={() => setAdding(true)}>
                {t("docs.add.action")}
              </Button>
            </>
          ) : undefined
        }
        footer={
          <Button variant="ghost" onClick={() => setShowHidden((s) => !s)}>
            {showHidden ? t("files.hideHidden") : t("files.showHidden")}
          </Button>
        }
      >
        <AddDocumentDialog
          anchor={{ record: "deal", id: dealId }}
          open={adding}
          onClose={() => setAdding(false)}
        />
        <PanelBody>
          <div ref={listRegion} tabIndex={-1}>
            <SurfaceState
              loadingLabel={t("files.title")}
              state={state}
              emptyLabel={t("files.empty")}
              detail={
                state === "failed"
                  ? { onRetry: () => void query.refetch() }
                  : undefined
              }
            >
              <FilesGrid
                dealId={dealId}
                files={files}
                mayWrite={mayWrite}
                focusLanding={focusLanding}
                onOpenEmail={onOpenEmail}
              />
            </SurfaceState>
          </div>
        </PanelBody>
      </Panel>
    </>
  );
}

function FilesGrid({
  dealId,
  files,
  mayWrite,
  focusLanding,
  onOpenEmail,
}: Readonly<{
  dealId: string;
  files: readonly DealDocument[];
  mayWrite: boolean;
  focusLanding: () => HTMLElement | null;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  return (
    <AttachmentGrid>
      {files.map((doc) => (
        <li key={doc.attachment.id}>
          <AttachmentTile
            file={doc.attachment}
            onOpenEmail={onOpenEmail}
            receded={doc.hidden}
            note={
              // A mail's file names who sent the mail; a meeting's or a
              // task's names nothing; an upload says it was put on the deal.
              doc.hidden ? (
                <Badge>{t("files.hiddenBadge")}</Badge>
              ) : doc.attachment.origin ||
                doc.attachment.entity_type === "activity" ? null : (
                <span className="t-caption">
                  {t("dealfiles.origin.upload")}
                </span>
              )
            }
            verbs={
              mayWrite && (
                <FileVerbs
                  file={doc.attachment}
                  focusLanding={focusLanding}
                  extra={<HideItems dealId={dealId} doc={doc} />}
                />
              )
            }
          />
        </li>
      ))}
    </AttachmentGrid>
  );
}

// The deal-only verbs of a captured file: the deal stops listing it, and the
// file stays on its message. A hide runs at once with an Undo.
function HideItems({
  dealId,
  doc,
}: Readonly<{ dealId: string; doc: DealDocument }>) {
  const t = useT();
  const { hide, unhide } = useHideVerbs(dealId, doc.attachment.id);
  if (doc.attachment.entity_type !== "activity") return null;
  const hidden = doc.hidden;
  const verb = hidden ? unhide : hide;
  return (
    <Button
      variant="ghost"
      pending={verb.isPending}
      onClick={() => verb.mutate()}
    >
      {t(hidden ? "files.unhide" : "files.hide")}
    </Button>
  );
}

// Each write refreshes both views of the area (with and without hidden rows),
// since a hide moves a row from one to the other.
function useHideVerbs(dealId: string, attachmentId: string) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  // A hide or unhide moves the row within this deal. That changes its area,
  // the Deal Room picker (same key, without hidden rows), and the rooms that
  // say whether each entry still reaches the buyer.
  const refresh = () =>
    Promise.all(
      dealHideKeys(dealId).map((queryKey) =>
        queryClient.invalidateQueries({ queryKey }),
      ),
    );
  // A refused hide or Undo has no dialog, and the row has no error slot.
  const sayRefused = (error: Error) =>
    toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true });
  const hide = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.PUT("/deals/{id}/documents/{attachmentId}/hide", {
          params: { path: { id: dealId, attachmentId } },
        }),
        t,
      );
    },
    onError: sayRefused,
    onSuccess: async () => {
      await refresh();
      // `DELETE .../hide` restores the row as it was, so a hide asks nothing first.
      toast.show(t("dealfiles.hidden"), {
        action: undoAction(t("common.undo"), () => unhide.mutate()),
      });
    },
  });
  const unhide = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.DELETE("/deals/{id}/documents/{attachmentId}/hide", {
          params: { path: { id: dealId, attachmentId } },
        }),
        t,
      );
    },
    onError: sayRefused,
    onSuccess: async () => {
      await refresh();
      toast.show(t("dealfiles.unhidden"));
    },
  });
  return { hide, unhide };
}
