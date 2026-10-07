import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type RefObject,
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button, OverflowMenu } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelRow } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { undoAction, useToast } from "../design-system/toast";
import { formatDateAbbrev } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { AddDocumentDialog } from "./adddocument";
import { problemMessageOf, throwProblem } from "./common";
import "./dealfiles.css";

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
};

// Enough for a working set; the area is not a library somebody pages through.
const PAGE_LIMIT = 100;

export function dealDocumentsKey(dealId: string, includeHidden: boolean) {
  return ["deal-documents", dealId, includeHidden] as const;
}

export function DealFiles({ deal }: Readonly<{ deal: Deal }>) {
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
  const query = useQuery({
    queryKey: dealDocumentsKey(dealId, showHidden),
    queryFn: async () => {
      const { data, error } = await api.GET("/deals/{id}/documents", {
        params: {
          path: { id: dealId },
          query: { limit: PAGE_LIMIT, include_hidden: showHidden },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
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
          {files.map((doc) => (
            <FileRow
              key={doc.attachment.id}
              dealId={dealId}
              doc={doc}
              mayWrite={mayWrite}
              focusLanding={focusLanding}
            />
          ))}
        </SurfaceState>
      </div>
    </Panel>
  );
}

function FileRow({
  dealId,
  doc,
  mayWrite,
  focusLanding,
}: Readonly<{
  dealId: string;
  doc: DealDocument;
  mayWrite: boolean;
  focusLanding: () => HTMLElement | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const file = doc.attachment;
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const verbs = useFileVerbs(dealId, file.id);
  const side = useRef<HTMLDivElement | null>(null);
  useFocusLeavesWithMenu(side, focusLanding);
  return (
    <PanelRow
      className={doc.hidden ? "deal-file deal-file-hidden" : "deal-file"}
    >
      <div className="deal-file-main">
        <a
          className="link-button"
          href={`/v1/attachments/${file.id}`}
          download={file.filename}
        >
          {file.title || file.filename}
        </a>
        <p className="t-caption">
          {doc.origin
            ? t("files.origin", {
                who: doc.origin.counterparty_email ?? t("files.originUnknown"),
                when: formatDateAbbrev(
                  doc.origin.occurred_at,
                  locale,
                  recordZone,
                ),
              })
            : t("files.uploaded", {
                when: formatDateAbbrev(file.created_at, locale, recordZone),
              })}
        </p>
      </div>
      <div className="deal-file-side" ref={side}>
        {file.category ? (
          <Badge>{t(CATEGORY_LABELS[file.category])}</Badge>
        ) : null}
        {doc.hidden ? <Badge>{t("files.hiddenBadge")}</Badge> : null}
        {mayWrite ? (
          <FileMenu
            doc={doc}
            hide={verbs.hide}
            unhide={verbs.unhide}
            onDelete={() => setConfirmingDelete(true)}
          />
        ) : null}
      </div>
      <ConfirmModal
        open={confirmingDelete}
        onClose={() => setConfirmingDelete(false)}
        returnFocusTo={focusLanding}
        title={t("files.deleteTitle", { name: file.filename })}
        confirmLabel={t("files.delete")}
        confirmVariant="danger"
        pending={verbs.remove.isPending}
        error={
          verbs.remove.isError ? problemMessageOf(verbs.remove.error, t) : null
        }
        onConfirm={() =>
          verbs.remove.mutate(undefined, {
            onSuccess: () => setConfirmingDelete(false),
          })
        }
      >
        <p>{t("files.deleteBody")}</p>
      </ConfirmModal>
    </PanelRow>
  );
}

// The menu hands focus back to its trigger, and a hide that drops the row
// takes the trigger with it.
function useFocusLeavesWithMenu(
  menuSide: RefObject<HTMLElement | null>,
  focusLanding: () => HTMLElement | null,
) {
  useLayoutEffect(() => {
    const node = menuSide.current;
    return () => {
      if (node?.contains(document.activeElement)) {
        focusLanding()?.focus();
      }
    };
  }, [menuSide, focusLanding]);
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
  const captured = doc.origin !== undefined;
  return (
    <OverflowMenu
      label={t("files.rowActions", { name: doc.attachment.filename })}
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
  // Both spellings of "the deal's files": this area's own key and the one the
  // Deal Room's picker reads, so a delete here never leaves a ghost there.
  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ["deal-documents", dealId] }),
      queryClient.invalidateQueries({ queryKey: ["deal-attachments", dealId] }),
    ]);
  // A refused hide or Undo has no dialog, and the row has no error slot.
  const sayRefused = (error: Error) =>
    toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true });
  const hide = useMutation({
    mutationFn: async () => {
      const { error } = await api.PUT(
        "/deals/{id}/documents/{attachmentId}/hide",
        {
          params: { path: { id: dealId, attachmentId } },
        },
      );
      if (error) {
        throwProblem(error, t);
      }
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
      const { error } = await api.DELETE(
        "/deals/{id}/documents/{attachmentId}/hide",
        {
          params: { path: { id: dealId, attachmentId } },
        },
      );
      if (error) {
        throwProblem(error, t);
      }
    },
    onError: sayRefused,
    onSuccess: async () => {
      await refresh();
      toast.show(t("dealfiles.unhidden"));
    },
  });
  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE("/attachments/{id}", {
        params: { path: { id: attachmentId } },
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    onSuccess: refresh,
  });
  return { hide, unhide, remove };
}
