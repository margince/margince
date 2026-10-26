import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { FileOrigin } from "../../components/fileorigin";
import { Badge, Button, OverflowMenu } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable } from "../../design-system/datatable";
import { useFocusHandoff } from "../../design-system/focushandoff";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { undoAction, useToast } from "../../design-system/toast";
import { activityDay } from "../../format/activitywhen";
import { formatDateAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { attachmentAnchor } from "../../lib/attachmentlink";
import { useCanWriteRecord } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { AddDocumentDialog } from "../voice-knowledge/adddocument";
import "./dealfiles.css";
import { ROOM_DOCUMENTS_PREFIX } from "./dealroomdocuments.key";

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
type Category = NonNullable<DealDocument["attachment"]["category"]>;

const CATEGORY_LABELS: Record<Category, MessageKey> = {
  contract: "docs.category.contract",
  offer: "docs.category.offer",
  legal: "docs.category.legal",
  email_attachment: "docs.category.email",
  message_attachment: "docs.category.message",
  other: "docs.category.other",
  link: "docs.category.link",
};

// Enough for a working set; the area is not a library somebody pages through.
const PAGE_LIMIT = 100;

export function dealDocumentsKey(dealId: string, includeHidden: boolean) {
  return ["deal-documents", dealId, includeHidden] as const;
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
    <Panel
      title={t("files.title")}
      titleAction={
        mayWrite ? (
          <Button onClick={() => setAdding(true)}>
            {t("docs.add.action")}
          </Button>
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
            <FilesTable
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
  );
}

function FilesTable({
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
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <DataTable
      label={t("files.title")}
      fold
      rows={[...files]}
      rowKey={(doc) => doc.attachment.id}
      columns={[
        {
          key: "file",
          header: t("dealfiles.col.file"),
          fold: "title",
          render: (doc) => <FileName doc={doc} />,
        },
        {
          key: "category",
          header: t("dealfiles.col.category"),
          render: (doc) =>
            doc.attachment.category
              ? t(CATEGORY_LABELS[doc.attachment.category])
              : null,
        },
        {
          key: "origin",
          header: t("dealfiles.col.origin"),
          // A mail's file names who sent the mail, as the contact and company
          // lists do. A meeting's or a task's file names nothing; an upload
          // says it was put on the deal.
          render: (doc) =>
            doc.attachment.origin ? (
              <FileOrigin
                origin={doc.attachment.origin}
                onOpenEmail={onOpenEmail}
              />
            ) : doc.attachment.entity_type === "activity" ? null : (
              t("dealfiles.origin.upload")
            ),
        },
        {
          key: "added",
          header: t("dealfiles.col.added"),
          render: (doc) =>
            doc.attachment.origin
              ? activityDay(
                  doc.attachment.origin.occurred_at,
                  doc.attachment.origin.occurred_precision,
                  locale,
                  recordZone,
                  formatDateAbbrev,
                )
              : formatDateAbbrev(doc.attachment.created_at, locale, recordZone),
        },
        ...(mayWrite
          ? [
              {
                key: "actions",
                header: t("table.actions"),
                headerHidden: true,
                fold: "end" as const,
                render: (doc: DealDocument) => (
                  <FileVerbs
                    dealId={dealId}
                    doc={doc}
                    focusLanding={focusLanding}
                  />
                ),
              },
            ]
          : []),
      ]}
    />
  );
}

function FileName({ doc }: Readonly<{ doc: DealDocument }>) {
  const t = useT();
  const file = doc.attachment;
  return (
    <span className={doc.hidden ? "deal-file deal-file-hidden" : "deal-file"}>
      <a className="cell-link" {...attachmentAnchor(file)}>
        {file.title || file.filename}
      </a>
      {doc.hidden ? <Badge>{t("files.hiddenBadge")}</Badge> : null}
    </span>
  );
}

// A row's menu and the delete it asks about first. The landing takes focus
// when the row the menu stood in goes away.
function FileVerbs({
  dealId,
  doc,
  focusLanding,
}: Readonly<{
  dealId: string;
  doc: DealDocument;
  focusLanding: () => HTMLElement | null;
}>) {
  const t = useT();
  const file = doc.attachment;
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const verbs = useFileVerbs(dealId, file.id);
  const side = useRef<HTMLSpanElement | null>(null);
  // A ref, not state: the dialog reads it as it closes, before a re-render.
  const deleted = useRef(false);
  useFocusHandoff(side, focusLanding);
  return (
    <span className="cell-actions" ref={side}>
      <FileMenu
        doc={doc}
        hide={verbs.hide}
        unhide={verbs.unhide}
        onDelete={() => {
          deleted.current = false;
          setConfirmingDelete(true);
        }}
      />
      <ConfirmModal
        open={confirmingDelete}
        onClose={() => setConfirmingDelete(false)}
        returnFocusTo={() => (deleted.current ? focusLanding() : null)}
        title={t("files.deleteTitle", { name: file.filename })}
        confirmLabel={t("files.delete")}
        confirmVariant="danger"
        pending={verbs.remove.isPending}
        error={
          verbs.remove.isError ? problemMessageOf(verbs.remove.error, t) : null
        }
        onConfirm={() =>
          verbs.remove.mutate(undefined, {
            onSuccess: () => {
              deleted.current = true;
              setConfirmingDelete(false);
            },
          })
        }
      >
        <p>{t("files.deleteBody")}</p>
      </ConfirmModal>
    </span>
  );
}

// The row's verbs: a captured file can be hidden or shown again, an upload
// deleted. The menu is its own component so the row stays readable.
function FileMenu({
  doc,
  hide,
  unhide,
  onDelete,
}: Readonly<{
  doc: DealDocument;
  hide: ReturnType<typeof useFileVerbs>["hide"];
  unhide: ReturnType<typeof useFileVerbs>["unhide"];
  onDelete: () => void;
}>) {
  const t = useT();
  const captured = doc.attachment.entity_type === "activity";
  return (
    <OverflowMenu
      label={t("table.rowActions", { name: doc.attachment.filename })}
    >
      {captured && !doc.hidden ? (
        <Button
          variant="ghost"
          pending={hide.isPending}
          onClick={() => hide.mutate()}
        >
          {t("files.hide")}
        </Button>
      ) : null}
      {captured && doc.hidden ? (
        <Button
          variant="ghost"
          pending={unhide.isPending}
          onClick={() => unhide.mutate()}
        >
          {t("files.unhide")}
        </Button>
      ) : null}
      {!captured ? (
        <Button variant="ghost" onClick={onDelete}>
          {t("files.delete")}
        </Button>
      ) : null}
    </OverflowMenu>
  );
}

// The three writes a row offers. Each refreshes both views of the area (with
// and without hidden rows), since a hide moves a row from one to the other.
function useFileVerbs(dealId: string, attachmentId: string) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  // Both keys of the deal's files: this area's own and the Deal Room picker's,
  // so a delete here leaves no ghost there. The room's own list goes too,
  // because it says whether each entry still reaches the buyer.
  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ["deal-documents", dealId] }),
      queryClient.invalidateQueries({ queryKey: ["deal-attachments", dealId] }),
      queryClient.invalidateQueries({ queryKey: ROOM_DOCUMENTS_PREFIX }),
    ]);
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
  const remove = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.DELETE("/attachments/{id}", {
          params: { path: { id: attachmentId } },
        }),
        t,
      );
    },
    onSuccess: refresh,
  });
  return { hide, unhide, remove };
}
