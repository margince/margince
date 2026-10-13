import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  OverflowMenu,
  SectionHeader,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { ConfirmModal } from "../design-system/confirmmodal";
import { IconAction } from "../design-system/iconaction";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
  PanelRow,
} from "../design-system/panel";
import { QueryGate, WriteRefused } from "../design-system/querystates";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem, unwrap } from "../lib/problem";
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

function useCreateDocumentSet() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { name: string; topicStatement: string }) => {
      unwrap(
        await api.POST("/knowledge/corpora", {
          body: { name: vars.name, topic_statement: vars.topicStatement },
        }),
      );
    },
    onSuccess: () => client.invalidateQueries({ queryKey: SETS_KEY }),
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

  // No empty state: every installation is filed with the shipped handbook, so
  // "no document sets yet" could only ever hide a reconciliation that failed.
  return (
    <Panel
      title={
        <span id={titleId} tabIndex={-1}>
          {t("knowledge.title")}
        </span>
      }
    >
      <PanelBody>
        <PanelIntro>{t("knowledge.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("knowledge.title")}>
        {(data) =>
          data.items.map((set) => (
            <DocumentSet
              key={set.id}
              set={set}
              canManage={canManage}
              afterArchive={() => document.getElementById(titleId)}
            />
          ))
        }
      </QueryGate>
      {canManage ? <NewDocumentSet /> : null}
    </Panel>
  );
}

function Passages({ set }: Readonly<{ set: Corpus }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { coverage } = set;
  // A set being re-read says so in its own notice, so it takes no second badge.
  const state =
    coverage.embed_state === "complete" || set.reindexing
      ? null
      : NOT_SEARCHABLE[coverage.embed_state];
  return (
    <CellStack>
      <span className="t-num">
        {plural("knowledge.passages", coverage.chunks_total, {
          embedded: formatNumber(coverage.chunks_embedded, locale),
          count: formatNumber(coverage.chunks_total, locale),
        })}
      </span>
      {state && (
        <>
          <Badge tone={state.tone}>{t(state.label)}</Badge>
          <span className="t-caption">{t(state.reason)}</span>
        </>
      )}
    </CellStack>
  );
}

// One set as one record row: it opens its documents in place, under it.
function DocumentSet({
  set,
  canManage,
  afterArchive,
}: Readonly<{
  set: Corpus;
  canManage: boolean;
  // The archived row and its menu leave with the refetch; focus lands here.
  afterArchive: () => HTMLElement | null;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const archived = useRef(false);
  const archive = useArchiveDocumentSet(() => {
    archived.current = true;
  });
  const documentsId = useId();
  const toggleId = useId();
  const toggle = () => setOpen((was) => !was);
  return (
    <>
      <PanelRow record onOpen={toggle} className="knowledge-set">
        <span className="knowledge-set-name">
          <CellStack>
            <span className="t-name">{set.name}</span>
            <span className="t-caption">{set.topic_statement}</span>
          </CellStack>
        </span>
        <span className="knowledge-set-documents t-num">
          {plural("knowledge.documentCount", set.coverage.documents_total, {
            count: formatNumber(set.coverage.documents_total, locale),
          })}
        </span>
        <span className="knowledge-set-passages">
          <Passages set={set} />
        </span>
        {/* A shipped set is filed again by the next update, so archiving it undoes nothing. */}
        {canManage && !set.shipped && (
          <span className="knowledge-set-menu cell-actions">
            <OverflowMenu label={t("table.rowActions", { name: set.name })}>
              <Button
                aria-haspopup="dialog"
                onClick={() => {
                  archive.reset();
                  archived.current = false;
                  setConfirming(true);
                }}
              >
                {t("knowledge.archive")}
              </Button>
            </OverflowMenu>
          </span>
        )}
        <span className="knowledge-set-open" id={toggleId}>
          <IconAction
            label={t("knowledge.documentsIn", { set: set.name })}
            icon={<ChevronDown aria-hidden className="expander-chevron" />}
            disclosure={{ expanded: open, controls: documentsId }}
            onClick={toggle}
          />
        </span>
      </PanelRow>
      {set.reindexing && (
        <PanelBody>
          <ReindexingNotice />
        </PanelBody>
      )}
      {/* No wrapper around the open list: its table bleeds only as a child of
          the panel. The empty stand-in keeps the chevron's target resolvable. */}
      {open ? (
        <>
          <PanelGroupHead
            id={documentsId}
            level="h3"
            title={t("knowledge.documentsIn", { set: set.name })}
          />
          <DocumentList
            corpusId={set.id}
            setName={set.name}
            setState={set.coverage.embed_state}
            shipped={set.shipped}
            canManage={canManage}
            afterDelete={() =>
              document.getElementById(toggleId)?.querySelector("button") ?? null
            }
          />
        </>
      ) : (
        <div id={documentsId} hidden />
      )}
      <ConfirmModal
        open={confirming}
        title={t("knowledge.archiveConfirm.title", { name: set.name })}
        confirmLabel={t("knowledge.archive")}
        pending={archive.isPending}
        error={
          archive.isError
            ? shippedRefusal(archive.error, t, ARCHIVE_SHIPPED)
            : null
        }
        onClose={() => setConfirming(false)}
        returnFocusTo={() => (archived.current ? afterArchive() : null)}
        onConfirm={() =>
          archive.mutate(
            { id: set.id },
            { onSuccess: () => setConfirming(false) },
          )
        }
      >
        <p>{t("knowledge.archiveConfirm.body")}</p>
      </ConfirmModal>
    </>
  );
}

function NewDocumentSet() {
  const t = useT();
  const [name, setName] = useState("");
  const [topic, setTopic] = useState("");
  const create = useCreateDocumentSet();
  const ready = name.trim() !== "" && topic.trim() !== "";

  return (
    <PanelBody className="form-stack">
      <SectionHeader level={3} title={t("knowledge.new.title")} />
      <Field label={t("knowledge.new.name")}>
        {(control) => (
          <TextInput
            {...control}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        )}
      </Field>
      {/* The topic statement is quoted back verbatim in a refusal, so it is
          read by a contact at their least patient moment. The hint says to write
          a sentence rather than a label, because "Handbook" tells a reader who
          just got a refusal nothing at all. */}
      <Field
        label={t("knowledge.new.topic")}
        hint={t("knowledge.new.topicHint")}
      >
        {(control) => (
          <Textarea
            {...control}
            value={topic}
            onChange={(event) => setTopic(event.target.value)}
          />
        )}
      </Field>
      <div className="form-actions">
        <Button
          disabled={!ready || create.isPending}
          onClick={() =>
            create.mutate(
              { name: name.trim(), topicStatement: topic.trim() },
              {
                onSuccess: () => {
                  setName("");
                  setTopic("");
                },
              },
            )
          }
        >
          {t("knowledge.new.submit")}
        </Button>
      </div>
      <WriteRefused titleKey="knowledge.new.failed" error={create.error} />
    </PanelBody>
  );
}
