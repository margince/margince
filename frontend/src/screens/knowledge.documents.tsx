// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
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
import { PanelBody } from "../design-system/panel";
import { forReader } from "../format/collate";
import { formatBytes, formatDate, formatNumber } from "../format/format";
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
import { problemMessageOf, QueryStates, throwProblem, unwrap } from "./common";
import { type UploadRefusal, UploadRefusals } from "./knowledge.notices";

type CorpusDocument = components["schemas"]["KnowledgeDocument"];

export const SETS_KEY = ["knowledge-corpora"];

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

// Multipart by hand: the generated client serializes JSON bodies only.
async function uploadDocument(vars: { corpusId: string; file: File }) {
  const body = new FormData();
  body.append("file", vars.file);
  const response = await fetch(
    `/v1/knowledge/corpora/${encodeURIComponent(vars.corpusId)}/documents`,
    { method: "POST", body, credentials: "include" },
  );
  const payload = await response.json().catch(() => undefined);
  if (!response.ok) {
    throwProblem(payload);
  }
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

function documentColumns({
  t,
  plural,
  locale,
  zone,
  canManage,
  onDelete,
}: Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
  canManage: boolean;
  onDelete: (doc: CorpusDocument) => void;
}>): DataTableColumn<CorpusDocument>[] {
  const columns: DataTableColumn<CorpusDocument>[] = [
    {
      key: "name",
      header: t("knowledge.col.name"),
      grow: true,
      render: (doc) => doc.filename,
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
      render: (doc) => formatDate(doc.created_at, locale, zone),
      folded: (doc) =>
        t("knowledge.addedOn", {
          date: formatDate(doc.created_at, locale, zone),
        }),
    },
    {
      key: "status",
      header: t("knowledge.col.status"),
      fold: "end",
      render: (doc) => {
        const badge = INGEST_BADGE[doc.ingest_status];
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
    },
  ];
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
      render: (doc) => (
        <span className="cell-actions">
          <OverflowMenu label={t("table.rowActions", { name: doc.filename })}>
            <Button aria-haspopup="dialog" onClick={() => onDelete(doc)}>
              {t("knowledge.deleteDocument")}
            </Button>
          </OverflowMenu>
        </span>
      ),
    },
  ];
}

/** One set's documents, A to Z, standing straight in the panel so they bleed. */
export function DocumentList({
  corpusId,
  setName,
  canManage,
}: Readonly<{ corpusId: string; setName: string; canManage: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const query = useDocuments(corpusId);
  const remove = useDocumentWrite(corpusId, deleteDocument);
  const [deleting, setDeleting] = useState<CorpusDocument | null>(null);
  const label = t("knowledge.documentsIn", { set: setName });
  const documents = [...(query.data?.items ?? [])].sort((a, b) =>
    forReader(a.filename, b.filename, locale),
  );
  return (
    <>
      {documents.length > 0 ? (
        <DataTable
          bleed
          fold
          label={label}
          columns={documentColumns({
            t,
            plural,
            locale,
            zone: viewerZone(),
            canManage,
            onDelete: (doc) => {
              remove.reset();
              setDeleting(doc);
            },
          })}
          rows={documents}
          rowKey={(doc) => doc.id}
        />
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={label}>
            <EmptyState>{t("knowledge.noDocuments")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      {canManage && query.isSuccess && (
        <PanelBody>
          <UploadDocument corpusId={corpusId} />
        </PanelBody>
      )}
      <ConfirmModal
        open={deleting !== null}
        title={t("knowledge.deleteConfirm.title", {
          name: deleting?.filename ?? "",
        })}
        confirmLabel={t("knowledge.deleteDocument")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onClose={() => setDeleting(null)}
        onConfirm={() => {
          if (deleting) {
            remove.mutate(
              { id: deleting.id },
              { onSuccess: () => setDeleting(null) },
            );
          }
        }}
      >
        <p>{t("knowledge.deleteConfirm.body")}</p>
      </ConfirmModal>
    </>
  );
}

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
          message: problemMessageOf(err, t),
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
