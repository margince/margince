import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useLastSubject } from "../design-system/presence";
import { QueryGate } from "../design-system/querystates";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCan, useCanWrite } from "../lib/capability";
import { throwProblem, unwrap } from "../lib/problem";
import { CreateAction } from "./create";
import {
  DocumentList,
  INGEST_POLL_MS,
  NOT_SEARCHABLE,
  SETS_KEY,
  shippedRefusal,
} from "./knowledge.documents";
import { ReindexingNotice } from "./knowledge.notices";
import "./knowledge.css";

// The document sets a workspace can be asked questions of.
//
// A NOTE ON THE WORD. The API calls these "corpora", and this screen
// deliberately does not. "Corpus" already means something else to a reader of
// this product — the VOICE corpus, the writing samples that teach drafts to
// sound like them — and it is on screen in onboarding, in the voice card and in
// the compose surface. Two things called a corpus, in one settings area, would
// be the same defect as the two tabs both called History. So the wire keeps
// `corpus` and the reader gets "document set", and the two are not to be
// reconciled by renaming the screen.
//
// Admin/ops only for every write, on the `knowledge_corpus` grant. A reader who
// may ask but not administer sees the sets and no verbs, which is the seeded
// posture: read is the ask, and every role that reads records holds it.

type Corpus = components["schemas"]["KnowledgeCorpus"];

function useDocumentSets(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: SETS_KEY,
    // A rebind sweep moves these counts with no document changing status.
    // An ordinary ingest refreshes this list from the documents instead.
    refetchInterval: (query) =>
      query.state.data?.items?.some((set) => set.reindexing)
        ? INGEST_POLL_MS
        : false,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/knowledge/corpora");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// `onArchived` runs before the refetch that unmounts the archived row and its dialog.
function useArchiveDocumentSet(onArchived: () => void) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }) => {
      unwrap(
        await api.DELETE("/knowledge/corpora/{id}", {
          params: { path: { id: vars.id } },
        }),
      );
    },
    onSuccess: () => {
      onArchived();
      return client.invalidateQueries({ queryKey: SETS_KEY });
    },
  });
}

const ARCHIVE_SHIPPED = new Map<string, MessageKey>([
  ["corpus_shipped", "knowledge.archiveShipped"],
]);

export function KnowledgeCard() {
  const t = useT();
  const canSee = useCan("knowledge_corpus", "read");
  const canManage = useCanWrite("knowledge_corpus", "create");
  const query = useDocumentSets(canSee);
  const titleId = useId();

  if (!canSee) {
    // Withheld, not absent: an empty card would claim the workspace has none.
    return (
      <Panel title={t("knowledge.title")}>
        <PanelBody>
          <PanelIntro>{t("knowledge.sub")}</PanelIntro>
          <EmptyState>{t("knowledge.withheld")}</EmptyState>
        </PanelBody>
      </Panel>
    );
  }

  return (
    <Panel
      title={
        <span id={titleId} tabIndex={-1}>
          {t("knowledge.title")}
        </span>
      }
      titleAction={canManage ? <NewDocumentSet /> : undefined}
    >
      <PanelBody>
        <PanelIntro>{t("knowledge.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("knowledge.title")}>
        {(data) =>
          data.items.length === 0 ? (
            <PanelBody>
              <EmptyState>{t("knowledge.empty")}</EmptyState>
            </PanelBody>
          ) : (
            <DocumentSets
              sets={data.items}
              canManage={canManage}
              afterArchive={() => document.getElementById(titleId)}
            />
          )
        }
      </QueryGate>
    </Panel>
  );
}

function setRowTestId(set: Corpus): string {
  return `knowledge-set-${set.id}`;
}

// The set's searchability as one badge; the figures and the reason are in its detail.
function SearchableBadge({ set }: Readonly<{ set: Corpus }>) {
  const t = useT();
  if (set.reindexing) {
    return <Badge tone="info">{t("knowledge.reindexingBadge")}</Badge>;
  }
  const state = set.coverage.embed_state;
  if (state === "complete") {
    return <Badge tone="success">{t("knowledge.searchable")}</Badge>;
  }
  return (
    <Badge tone={NOT_SEARCHABLE[state].tone}>
      {t(NOT_SEARCHABLE[state].label)}
    </Badge>
  );
}

function DocumentSets({
  sets,
  canManage,
  afterArchive,
}: Readonly<{
  sets: readonly Corpus[];
  canManage: boolean;
  // The archived row and its menu leave with the refetch; focus lands here.
  afterArchive: () => HTMLElement | null;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [confirming, setConfirming] = useState<Corpus | null>(null);
  const asked = useLastSubject(confirming);
  const archived = useRef(false);
  const archive = useArchiveDocumentSet(() => {
    archived.current = true;
  });
  const columns: DataTableColumn<Corpus>[] = [
    {
      key: "name",
      header: t("knowledge.col.name"),
      grow: true,
      render: (set) => <span className="t-name">{set.name}</span>,
    },
    {
      key: "documents",
      header: t("knowledge.col.documents"),
      align: "end",
      render: (set) => formatNumber(set.coverage.documents_total, locale),
      folded: (set) =>
        plural("knowledge.documentCount", set.coverage.documents_total, {
          count: formatNumber(set.coverage.documents_total, locale),
        }),
    },
    {
      key: "searchable",
      header: t("knowledge.col.searchable"),
      fold: "end",
      render: (set) => <SearchableBadge set={set} />,
    },
  ];
  if (canManage) {
    columns.push({
      key: "actions",
      header: t("knowledge.col.actions"),
      headerHidden: true,
      fold: "end",
      // A shipped set is filed again by the next update, so archiving it undoes nothing.
      render: (set) =>
        set.shipped ? null : (
          <span className="cell-actions">
            <OverflowMenu label={t("table.rowActions", { name: set.name })}>
              <Button
                aria-haspopup="dialog"
                onClick={() => {
                  archive.reset();
                  archived.current = false;
                  setConfirming(set);
                }}
              >
                {t("knowledge.archive")}
              </Button>
            </OverflowMenu>
          </span>
        ),
    });
  }
  return (
    <>
      <DataTable
        bleed
        fold
        label={t("knowledge.title")}
        columns={columns}
        rows={[...sets]}
        rowKey={(set) => set.id}
        rowTestId={setRowTestId}
        detail={{
          header: t("knowledge.col.detail"),
          toggleLabel: (set) => t("knowledge.documentsIn", { set: set.name }),
          single: true,
          render: (set) => <SetDetail set={set} canManage={canManage} />,
        }}
      />
      <ConfirmModal
        open={confirming !== null}
        title={t("knowledge.archiveConfirm.title", { name: asked?.name ?? "" })}
        confirmLabel={t("knowledge.archive")}
        pending={archive.isPending}
        error={
          archive.isError
            ? shippedRefusal(archive.error, t, ARCHIVE_SHIPPED)
            : null
        }
        onClose={() => setConfirming(null)}
        returnFocusTo={() => (archived.current ? afterArchive() : null)}
        onConfirm={() => {
          if (confirming) {
            archive.mutate(
              { id: confirming.id },
              { onSuccess: () => setConfirming(null) },
            );
          }
        }}
      >
        <p>{t("knowledge.archiveConfirm.body")}</p>
      </ConfirmModal>
    </>
  );
}

// What the set covers, how much of it answers, then its documents.
function SetDetail({
  set,
  canManage,
}: Readonly<{ set: Corpus; canManage: boolean }>) {
  const t = useT();
  const root = useRef<HTMLDivElement>(null);
  // The chevron that opened this detail: a deleted document's menu leaves with it.
  const toggle = () => {
    const cell = root.current?.closest("td[id]");
    return cell
      ? ([...document.querySelectorAll<HTMLElement>("[aria-controls]")].find(
          (control) => control.getAttribute("aria-controls") === cell.id,
        ) ?? null)
      : null;
  };
  const plural = usePlural();
  const { locale } = useLocale();
  const { coverage } = set;
  const state =
    coverage.embed_state === "complete" || set.reindexing
      ? null
      : NOT_SEARCHABLE[coverage.embed_state];
  return (
    <div className="knowledge-detail" ref={root}>
      <div className="knowledge-detail-head">
        <p>{set.topic_statement}</p>
        <p className="t-caption">
          {plural("knowledge.passages", coverage.chunks_total, {
            embedded: formatNumber(coverage.chunks_embedded, locale),
            count: formatNumber(coverage.chunks_total, locale),
          })}
        </p>
        {state && <p className="t-caption">{t(state.reason)}</p>}
      </div>
      {set.reindexing && <ReindexingNotice />}
      <DocumentList
        corpusId={set.id}
        setName={set.name}
        setState={coverage.embed_state}
        shipped={set.shipped}
        canManage={canManage}
        afterDelete={toggle}
      />
    </div>
  );
}

function NewDocumentSet() {
  const t = useT();
  return (
    <CreateAction
      label={t("knowledge.new.title")}
      invalidate={SETS_KEY[0]}
      screen="settings"
      stay
      testId="new-document-set"
      fields={[
        { key: "name", label: "knowledge.new.name", required: true },
        {
          key: "topic_statement",
          label: "knowledge.new.topic",
          type: "textarea",
          required: true,
          // Quoted back verbatim in a refusal, so it asks for a sentence, not a label.
          hint: t("knowledge.new.topicHint"),
        },
      ]}
      create={async (values) =>
        unwrap(
          await api.POST("/knowledge/corpora", {
            body: {
              name: (values.name ?? "").trim(),
              topic_statement: (values.topic_statement ?? "").trim(),
            },
          }),
        )
      }
    />
  );
}
