// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { FileDropzone } from "../design-system/filedropzone";
import { useLastSubject } from "../design-system/presence";
import { QueryStates } from "../design-system/querystates";
import { forReader } from "../format/collate";
import { formatBytes, formatDateAbbrev, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { MessageKey } from "../i18n/en";
import { postMultipart } from "../lib/multipart";
import {
  problemCodeOf,
  problemMessageOf,
  throwProblem,
  unwrap,
} from "../lib/problem";
import { type UploadRefusal, UploadRefusals } from "./knowledge.notices";

type CorpusDocument = components["schemas"]["KnowledgeDocument"];
type EmbedState = components["schemas"]["KnowledgeEmbedState"];

export const SETS_KEY = ["knowledge-corpora"] as const;

// Slow enough not to hammer the API, fast enough that a dropped file visibly
// changes state.
export const INGEST_POLL_MS = 2500;

function documentsKey(corpusId: string) {
  return ["knowledge-documents", corpusId];
}

// A failed ingest is final too: one bad file must not hold the poll open.
function settled(documents: readonly CorpusDocument[]): boolean {
  return documents.every(
    (doc) => doc.ingest_status !== "queued" && doc.ingest_status !== "running",
  );
}

function useDocuments(corpusId: string) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: documentsKey(corpusId),
    refetchInterval: (query) => {
      const items = query.state.data?.items;
      return items && settled(items) ? false : INGEST_POLL_MS;
    },
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/knowledge/corpora/{id}/documents",
        { params: { path: { id: corpusId } } },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });

  // The set's coverage comes from another query, so it is refreshed at the
  // moment this list settles, where the transition is observable.
  const items = query.data?.items;
  const done = items !== undefined && settled(items);
  useEffect(() => {
    if (done) {
      void client.invalidateQueries({ queryKey: SETS_KEY });
    }
  }, [done, client]);

  return query;
}

// An upload or a delete moves both lists: the documents and the set's counts.
function useDocumentWrite<Vars>(
  corpusId: string,
  write: (vars: Vars) => Promise<void>,
) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: write,
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: documentsKey(corpusId) });
      await client.invalidateQueries({ queryKey: SETS_KEY });
    },
  });
}

async function uploadDocument(vars: { corpusId: string; file: File }) {
  await postMultipart(
    `/v1/knowledge/corpora/${encodeURIComponent(vars.corpusId)}/documents`,
    { file: vars.file },
  );
}

async function deleteDocument(vars: { id: string }) {
  unwrap(
    await api.DELETE("/knowledge/documents/{id}", {
      params: { path: { id: vars.id } },
    }),
  );
}

// Only the states a reader must wait on or act on take a badge; "done" is quiet.
const INGEST_BADGE: Partial<
  Record<
    CorpusDocument["ingest_status"],
    Readonly<{ label: MessageKey; tone?: "danger" | "info" }>
  >
> = {
  queued: { label: "knowledge.ingest.queued", tone: "info" },
  running: { label: "knowledge.ingest.running", tone: "info" },
  failed: { label: "knowledge.ingest.failed", tone: "danger" },
};

// Why passages are not all searchable, in the reader's words, for a set or one document.
export const NOT_SEARCHABLE: Record<
  Exclude<EmbedState, "complete">,
  Readonly<{ label: MessageKey; reason: MessageKey; tone: "warning" | "info" }>
> = {
  no_embedding_model: {
    label: "knowledge.notSearchable",
    reason: "knowledge.reason.noEmbeddingModel",
    tone: "warning",
  },
  indexing: {
    label: "knowledge.indexing",
    reason: "knowledge.reason.indexing",
    tone: "info",
  },
};

// An importing or failed document already wears that badge.
function documentBadge(
  doc: CorpusDocument,
  setSpeaksForAll: boolean,
): Readonly<{
  label: MessageKey;
  tone?: "danger" | "info" | "warning";
}> | null {
  const ingest = INGEST_BADGE[doc.ingest_status];
  if (ingest) return ingest;
  return doc.embed_state === "complete" || setSpeaksForAll
    ? null
    : NOT_SEARCHABLE[doc.embed_state];
}

function documentName(doc: CorpusDocument): string {
  return doc.title ?? doc.filename;
}

// "approvals.md" names "Approvals": a caption repeating the title says nothing.
function sameName(title: string, filename: string): boolean {
  const words = (text: string) =>
    text
      .replace(/\.[a-z0-9]+$/i, "")
      .replace(/[-_\s]+/g, " ")
      .trim()
      .toLowerCase();
  return words(title) === words(filename);
}

function documentColumns({
  t,
  plural,
  locale,
  zone,
  setSpeaksForAll,
  anyBadge,
  canManage,
  onDelete,
}: Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
  setSpeaksForAll: boolean;
  // A status column every row leaves empty is a heading over nothing.
  anyBadge: boolean;
  canManage: boolean;
  onDelete: (doc: CorpusDocument) => void;
}>): DataTableColumn<CorpusDocument>[] {
  const columns: DataTableColumn<CorpusDocument>[] = [
    {
      key: "name",
      header: t("knowledge.col.name"),
      grow: true,
      render: (doc) =>
        doc.title === null || sameName(doc.title, doc.filename) ? (
          documentName(doc)
        ) : (
          <CellStack>
            <span>{doc.title}</span>
            <span className="t-caption">{doc.filename}</span>
          </CellStack>
        ),
    },
    {
      key: "passages",
      header: t("knowledge.col.passages"),
      align: "end",
      render: (doc) =>
        doc.chunk_count === undefined
          ? null
          : formatNumber(doc.chunk_count, locale),
      folded: (doc) =>
        doc.chunk_count === undefined
          ? undefined
          : plural("knowledge.passageCount", doc.chunk_count, {
              count: formatNumber(doc.chunk_count, locale),
            }),
    },
    {
      key: "size",
      header: t("knowledge.col.size"),
      align: "end",
      render: (doc) => formatBytes(doc.byte_size, locale),
    },
    {
      key: "added",
      header: t("knowledge.col.added"),
      render: (doc) => formatDateAbbrev(doc.created_at, locale, zone),
      folded: (doc) =>
        t("knowledge.addedOn", {
          date: formatDateAbbrev(doc.created_at, locale, zone),
        }),
    },
  ];
  if (anyBadge) {
    columns.push({
      key: "status",
      header: t("knowledge.col.status"),
      fold: "end",
      render: (doc) => {
        const badge = documentBadge(doc, setSpeaksForAll);
        return (
          badge && (
            <CellStack>
              <Badge tone={badge.tone}>{t(badge.label)}</Badge>
              {doc.ingest_detail && (
                <span className="t-caption">{doc.ingest_detail}</span>
              )}
            </CellStack>
          )
        );
      },
    });
  }
  if (!canManage) {
    return columns;
  }
  return [
    ...columns,
    {
      key: "actions",
      header: t("knowledge.col.actions"),
      headerHidden: true,
      fold: "end",
      // A shipped page is filed again by the next update, so deleting it undoes nothing.
      render: (doc) =>
        doc.shipped ? null : (
          <span className="cell-actions">
            <OverflowMenu
              label={t("table.rowActions", { name: documentName(doc) })}
            >
              <Button aria-haspopup="dialog" onClick={() => onDelete(doc)}>
                {t("knowledge.deleteDocument")}
              </Button>
            </OverflowMenu>
          </span>
        ),
    },
  ];
}

/** One set's documents, A to Z, in the set's open detail. */
export function DocumentList({
  corpusId,
  setName,
  setState,
  shipped,
  canManage,
  afterDelete,
}: Readonly<{
  corpusId: string;
  setName: string;
  setState: EmbedState;
  // Only Margince files pages in a set it ships.
  shipped: boolean;
  canManage: boolean;
  // A deleted document takes its row's menu with it; focus lands here instead.
  afterDelete: () => HTMLElement | null;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const query = useDocuments(corpusId);
  const remove = useDocumentWrite(corpusId, deleteDocument);
  const [deleting, setDeleting] = useState<CorpusDocument | null>(null);
  const asked = useLastSubject(deleting);
  const deleted = useRef(false);
  const label = t("knowledge.documentsIn", { set: setName });
  const documents = [...(query.data?.items ?? [])].sort((a, b) =>
    forReader(documentName(a), documentName(b), locale),
  );
  // The set states its searchability once, with its reason. Rows repeat it only
  // when some differ, so the one stale document can be found.
  const setSpeaksForAll =
    setState !== "complete" &&
    documents.every((doc) => doc.embed_state === setState);
  const anyBadge = documents.some(
    (doc) => documentBadge(doc, setSpeaksForAll) !== null,
  );
  return (
    <>
      {documents.length > 0 ? (
        <DataTable
          fold
          label={label}
          columns={documentColumns({
            t,
            plural,
            locale,
            zone: viewerZone(),
            setSpeaksForAll,
            anyBadge,
            canManage,
            onDelete: (doc) => {
              remove.reset();
              deleted.current = false;
              setDeleting(doc);
            },
          })}
          rows={documents}
          rowKey={(doc) => doc.id}
        />
      ) : (
        <QueryStates query={query} pendingLabel={label}>
          <EmptyState>{t("knowledge.noDocuments")}</EmptyState>
        </QueryStates>
      )}
      {canManage && !shipped && query.isSuccess && (
        <UploadDocument corpusId={corpusId} />
      )}
      <ConfirmModal
        open={deleting !== null}
        title={t("knowledge.deleteConfirm.title", {
          name: asked ? documentName(asked) : "",
        })}
        confirmLabel={t("knowledge.deleteDocument")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={
          remove.isError
            ? shippedRefusal(remove.error, t, DELETE_SHIPPED)
            : null
        }
        onClose={() => setDeleting(null)}
        returnFocusTo={() => (deleted.current ? afterDelete() : null)}
        onConfirm={() => {
          if (deleting) {
            remove.mutate(
              { id: deleting.id },
              {
                onSuccess: () => {
                  deleted.current = true;
                  setDeleting(null);
                },
              },
            );
          }
        }}
      >
        <p>{t("knowledge.deleteConfirm.body")}</p>
      </ConfirmModal>
    </>
  );
}

// A write to what ships with Margince is refused in catalog copy, by code. The
// list read can predate the shipped flag the press meets.
export function shippedRefusal(
  error: unknown,
  t: Translator,
  copy: ReadonlyMap<string, MessageKey>,
): string {
  const key = copy.get(problemCodeOf(error) ?? "");
  return key ? t(key) : problemMessageOf(error, t);
}

const DELETE_SHIPPED = new Map<string, MessageKey>([
  ["document_shipped", "knowledge.deleteShipped"],
]);
const UPLOAD_SHIPPED = new Map<string, MessageKey>([
  ["corpus_shipped", "knowledge.uploadShipped"],
]);

// Two distinct files agreeing on all three are the same bytes for every
// purpose here, and the server would refuse both identically.
function fileIdentity(file: File): string {
  return `${file.name}:${file.size}:${file.lastModified}`;
}

function UploadDocument({ corpusId }: Readonly<{ corpusId: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [files, setFiles] = useState<readonly File[]>([]);
  const [refusals, setRefusals] = useState<readonly UploadRefusal[]>([]);
  const [sending, setSending] = useState(false);
  const upload = useDocumentWrite(corpusId, uploadDocument);

  // One at a time: each upload locks the same corpus row, and the refusals
  // keep the order the reader dropped the files in.
  const send = async () => {
    setSending(true);
    setRefusals([]);
    const refused: UploadRefusal[] = [];
    const accepted: File[] = [];
    for (const file of files) {
      try {
        await upload.mutateAsync({ corpusId, file });
        accepted.push(file);
      } catch (err) {
        // One bad file does not abandon the rest of the drop.
        refused.push({
          id: fileIdentity(file),
          filename: file.name,
          message: shippedRefusal(err, t, UPLOAD_SHIPPED),
        });
      }
    }
    // From the latest held list: a file added mid-send must not be dropped.
    setFiles((held) => held.filter((one) => !accepted.includes(one)));
    setRefusals(refused);
    setSending(false);
  };

  return (
    <div className="form-stack">
      <FileDropzone
        label={t("knowledge.upload.label")}
        hint={t("knowledge.upload.hint")}
        emptyLabel={t("knowledge.upload.empty")}
        multiple
        files={files}
        // A second drop adds every file, names included: the server refuses a
        // duplicate by checksum and names the document holding those bytes.
        onPick={(picked) => setFiles((held) => [...held, picked])}
      />
      <div className="form-actions">
        <Button disabled={files.length === 0 || sending} onClick={send}>
          {/* With nothing chosen the button names the act, not a count of none. */}
          {plural("knowledge.upload.submit", files.length || 1, {
            count: formatNumber(files.length, locale),
          })}
        </Button>
      </div>
      <UploadRefusals refusals={refusals} />
    </div>
  );
}
