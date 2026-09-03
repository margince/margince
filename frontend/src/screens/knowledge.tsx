import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  SectionHeader,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { FileDropzone } from "../design-system/filedropzone";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

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
type CorpusDocument = components["schemas"]["KnowledgeDocument"];

const SETS_KEY = ["knowledge-corpora"];

function documentsKey(corpusId: string) {
  return ["knowledge-documents", corpusId];
}

function useDocumentSets(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: SETS_KEY,
    // A rebind sweep moves these counts with no document changing status, so
    // this list has one in-flight signal of its own. The ordinary ingest case
    // is driven from the document list instead, which is the thing that knows
    // when it is finished — see useDocuments.
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
      const { error } = await api.POST("/knowledge/corpora", {
        body: { name: vars.name, topic_statement: vars.topicStatement },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => client.invalidateQueries({ queryKey: SETS_KEY }),
  });
}

function useArchiveDocumentSet() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }) => {
      const { error } = await api.DELETE("/knowledge/corpora/{id}", {
        params: { path: { id: vars.id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => client.invalidateQueries({ queryKey: SETS_KEY }),
  });
}

// How often the list asks again while an ingest is still moving. Slow enough
// that a set of twenty documents is not a request every second, fast enough
// that a reader watching a file they just dropped sees it change without
// wondering whether the page is broken.
const INGEST_POLL_MS = 2500;

// settled is a document whose status will not change on its own. Only `queued`
// and `running` are waiting on a worker; `done` and `failed` are both final,
// and a failed one must NOT hold the poll open or a set with one bad file
// polls for as long as the tab is open.
function settled(documents: readonly CorpusDocument[]): boolean {
  return documents.every(
    (doc) => doc.ingest_status !== "queued" && doc.ingest_status !== "running",
  );
}

function useDocuments(corpusId: string, enabled: boolean) {
  const client = useQueryClient();
  const query = useQuery({
    enabled,
    queryKey: documentsKey(corpusId),
    // Ingest is asynchronous, so the status shown at upload is a snapshot of
    // one instant. Without this the badges sit on "Waiting to be read" until a
    // reload — the screen reporting a state the server left minutes ago, which
    // is worse than showing nothing because it looks live.
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

  // The set's own line — "10 documents, N of M passages searchable" — is served
  // by a DIFFERENT query, so it does not move when a document finishes being
  // read. Refreshed here at the moment THIS list settles, because that is where
  // the transition is observable: the alternative is polling the sets list on a
  // condition it cannot see, which would either never stop or never start.
  const items = query.data?.items;
  const done = items !== undefined && settled(items);
  useEffect(() => {
    if (done) {
      void client.invalidateQueries({ queryKey: SETS_KEY });
    }
  }, [done, client]);

  return query;
}

function useUploadDocument(corpusId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({
      corpusId,
      file,
    }: {
      corpusId: string;
      file: File;
    }) => {
      // Sent as multipart by hand rather than through the typed client: the
      // generated client serializes JSON bodies, and this endpoint takes a
      // file part. The linkedin import does the same, for the same reason.
      const body = new FormData();
      body.append("file", file);
      const response = await fetch(
        `/v1/knowledge/corpora/${encodeURIComponent(corpusId)}/documents`,
        { method: "POST", body, credentials: "include" },
      );
      const payload = await response.json().catch(() => undefined);
      if (!response.ok) {
        throwProblem(payload);
      }
    },
    // An upload changes BOTH lists: the document appears here, and the set's
    // coverage counts above it move. They hold separate cache keys, so
    // invalidating one leaves the page contradicting itself until a reload.
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: documentsKey(corpusId) });
      await client.invalidateQueries({ queryKey: SETS_KEY });
    },
  });
}

function useDeleteDocument(corpusId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }) => {
      const { error } = await api.DELETE("/knowledge/documents/{id}", {
        params: { path: { id: vars.id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: documentsKey(corpusId) });
      await client.invalidateQueries({ queryKey: SETS_KEY });
    },
  });
}

// ingestLabelKey names the copy for one ingest state.
//
// A switch rather than a template literal: `knowledge.ingest.${status}` is a
// string TypeScript cannot check against the message catalog, so a state whose
// copy nobody wrote would render its own key on screen. Spelled out, a missing
// one is a compile error.
type IngestLabelKey =
  | "knowledge.ingest.queued"
  | "knowledge.ingest.running"
  | "knowledge.ingest.failed"
  | "knowledge.ingest.done";

function ingestLabelKey(
  status: CorpusDocument["ingest_status"],
): IngestLabelKey {
  switch (status) {
    case "queued":
      return "knowledge.ingest.queued";
    case "running":
      return "knowledge.ingest.running";
    case "failed":
      return "knowledge.ingest.failed";
    default:
      return "knowledge.ingest.done";
  }
}

// ingestTone maps an ingest state onto the badge tone that says what a reader
// should DO about it. Only `failed` is bad news; `queued` and `running` are the
// same "wait" in two words, and `done` is the quiet one because a set where
// everything worked should not be a wall of green pills.
function ingestTone(
  status: CorpusDocument["ingest_status"],
): "danger" | "success" | undefined {
  if (status === "failed") {
    return "danger";
  }
  if (status === "done") {
    return "success";
  }
  // Queued and running are the same "wait" in two words, and neither is news.
  // An undefined tone is the Badge's own quiet ground, which is right: a set
  // where everything is proceeding should not be a wall of coloured pills.
  return undefined;
}

export function KnowledgeCard() {
  const t = useT();
  const canSee = useCan("knowledge_corpus", "read");
  const canManage = useCanWrite("knowledge_corpus", "create");
  const query = useDocumentSets(canSee);

  if (!canSee) {
    // Withheld, not absent. An empty card would claim this workspace has no
    // document sets — a statement about the DATA — when the truth is only that
    // they are not this reader's to administer.
    return (
      <Panel title={t("knowledge.title")}>
        <PanelBody>
          <p className="t-caption">{t("knowledge.sub")}</p>
          <EmptyState>{t("knowledge.withheld")}</EmptyState>
        </PanelBody>
      </Panel>
    );
  }

  return (
    <Panel title={t("knowledge.title")}>
      <PanelBody className="form-stack">
        <p className="t-caption">{t("knowledge.sub")}</p>
        <QueryGate query={query} pendingLabel={t("knowledge.title")}>
          {(data) => (
            <DocumentSetList sets={data.items} canManage={canManage} />
          )}
        </QueryGate>
      </PanelBody>
      {canManage ? <NewDocumentSet /> : null}
    </Panel>
  );
}

// No empty state, because the list is never empty. Every installation is filed
// with the operator handbook as its shipped corpus when the api boots, so
// "this workspace has no document sets" is not a state a reader can reach. An
// empty state here would be a message written for a screen nobody sees — and
// worse, the ONE way it could appear is a failed handbook reconciliation, where
// "no document sets yet" is exactly the wrong thing to say: it reads as normal,
// invites the reader to create one, and hides that something did not run.
function DocumentSetList({
  sets,
  canManage,
}: Readonly<{ sets: readonly Corpus[]; canManage: boolean }>) {
  return (
    <>
      {sets.map((set) => (
        <DocumentSetRow key={set.id} set={set} canManage={canManage} />
      ))}
    </>
  );
}

function DocumentSetRow({
  set,
  canManage,
}: Readonly<{ set: Corpus; canManage: boolean }>) {
  const t = useT();
  // Counts, so they are MAGNITUDES and take the reader's own notation.
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const archive = useArchiveDocumentSet();

  return (
    <PanelRow>
      <div className="form-stack">
        <SectionHeader
          level={3}
          title={set.name}
          sub={set.topic_statement}
          actions={
            canManage ? (
              <Button
                variant="ghost"
                onClick={() => setConfirming(true)}
                disabled={archive.isPending}
              >
                {t("knowledge.archive")}
              </Button>
            ) : undefined
          }
        />
        <p className="t-small">
          {t("knowledge.coverage", {
            embedded: formatNumber(set.coverage.chunks_embedded, locale),
            total: formatNumber(set.coverage.chunks_total, locale),
            documents: formatNumber(set.coverage.documents_total, locale),
          })}
        </p>
        {/* A set being re-read has nothing for the reader to do but wait, and
            saying so is different from saying it is not ready. */}
        {set.reindexing ? (
          <Callout tone="info">{t("knowledge.reindexing")}</Callout>
        ) : null}
        <Button variant="ghost" onClick={() => setOpen((was) => !was)}>
          {open ? t("knowledge.hideDocuments") : t("knowledge.showDocuments")}
        </Button>
        {open ? <DocumentList corpusId={set.id} canManage={canManage} /> : null}
        {archive.isError ? (
          <Callout tone="danger">{problemMessageOf(archive.error, t)}</Callout>
        ) : null}
      </div>
      <ConfirmModal
        open={confirming}
        title={t("knowledge.archiveConfirm.title")}
        confirmLabel={t("knowledge.archive")}
        onClose={() => setConfirming(false)}
        onConfirm={() => {
          archive.mutate({ id: set.id });
          setConfirming(false);
        }}
      >
        <p className="t-small">{t("knowledge.archiveConfirm.body")}</p>
      </ConfirmModal>
    </PanelRow>
  );
}

function DocumentList({
  corpusId,
  canManage,
}: Readonly<{ corpusId: string; canManage: boolean }>) {
  const t = useT();
  const query = useDocuments(corpusId, true);
  return (
    <QueryGate query={query} pendingLabel={t("knowledge.documents")}>
      {(data) => (
        <div className="form-stack">
          {data.items.length === 0 ? (
            <EmptyState>{t("knowledge.noDocuments")}</EmptyState>
          ) : (
            data.items.map((doc) => (
              <DocumentRow
                key={doc.id}
                corpusId={corpusId}
                doc={doc}
                canManage={canManage}
              />
            ))
          )}
          {canManage ? <UploadDocument corpusId={corpusId} /> : null}
        </div>
      )}
    </QueryGate>
  );
}

function DocumentRow({
  corpusId,
  doc,
  canManage,
}: Readonly<{
  corpusId: string;
  doc: CorpusDocument;
  canManage: boolean;
}>) {
  const t = useT();
  const [confirming, setConfirming] = useState(false);
  const remove = useDeleteDocument(corpusId);

  return (
    <div className="form-stack">
      <SectionHeader
        level={3}
        title={doc.filename}
        actions={
          canManage ? (
            <Button
              variant="ghost"
              onClick={() => setConfirming(true)}
              disabled={remove.isPending}
            >
              {t("knowledge.deleteDocument")}
            </Button>
          ) : undefined
        }
      />
      <Badge tone={ingestTone(doc.ingest_status)}>
        {t(ingestLabelKey(doc.ingest_status))}
      </Badge>
      {/* The reason a failed ingest carries. A set quietly short of a file
          nobody can name answers worse than an empty one, because it still
          answers. */}
      {doc.ingest_detail ? (
        <Callout tone="danger">{doc.ingest_detail}</Callout>
      ) : null}
      {remove.isError ? (
        <Callout tone="danger">{problemMessageOf(remove.error, t)}</Callout>
      ) : null}
      <ConfirmModal
        open={confirming}
        title={t("knowledge.deleteConfirm.title")}
        confirmLabel={t("knowledge.deleteDocument")}
        onClose={() => setConfirming(false)}
        onConfirm={() => {
          remove.mutate({ id: doc.id });
          setConfirming(false);
        }}
      >
        <p className="t-small">{t("knowledge.deleteConfirm.body")}</p>
      </ConfirmModal>
    </div>
  );
}

// One refused file out of several. Named, because "3 of 10 failed" leaves the
// reader to work out WHICH three by comparing two lists by eye.
//
// `id` is the file's OWN identity, not its position in the batch: two refused
// files may share a name now that both are kept, and a key that changes when
// the list is rebuilt would let React reuse the wrong message.
type UploadRefusal = Readonly<{
  id: string;
  filename: string;
  message: string;
}>;

// What distinguishes one picked file from another without opening it. Two
// distinct files agreeing on all three are the same bytes for every purpose
// this screen has — and the server would refuse both identically, so a
// collision could only ever merge two messages that read the same.
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
  const upload = useUploadDocument(corpusId);

  // Uploaded one at a time, not in parallel. Each file is a separate write
  // that takes a row lock on the same corpus, and firing ten at once turns a
  // queue into lock contention for no gain a reader would notice. It also
  // keeps the refusals in the order the reader dropped them.
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
        // One bad file does not abandon the rest: a reader who dropped ten
        // handbook pages and has one duplicate among them wants the nine.
        refused.push({
          id: fileIdentity(file),
          filename: file.name,
          message: problemMessageOf(err, t),
        });
      }
    }
    // Only what was refused stays in the zone, so the button retries exactly
    // the files that still need it rather than re-sending the whole drop.
    //
    // Computed from the LATEST held list rather than the one this send closed
    // over: the dropzone stays live while an upload runs, so a reader who adds
    // a file mid-send would otherwise have it discarded by this line — the
    // silent loss the pick handler above refuses to make.
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
        onPick={(picked) =>
          // A second drop ADDS, and adds EVERY file. Replacing would silently
          // discard the first drop, and a reader gathering files from two
          // folders has no way to tell that happened until the counts are
          // wrong — which is exactly what a filename filter here reintroduced.
          //
          // No name check, deliberately: the server refuses a duplicate by
          // CHECKSUM and names the document already holding those bytes. Two
          // files called notes.md from different folders are two documents this
          // product accepts, so a client that dropped the second would be
          // stricter than the thing it is a client of, and silent about it.
          setFiles((held) => [...held, picked])
        }
      />
      <div className="form-actions">
        <Button disabled={files.length === 0 || sending} onClick={send}>
          {/* Before anything is chosen the button names the ACT, not a count
              of nothing: "Add 0 documents" on a disabled button reads like a
              refusal rather than an invitation. One file and none both take the
              singular; the count only appears once there is one to state. */}
          {plural("knowledge.upload.submit", files.length || 1, {
            count: formatNumber(files.length, locale),
          })}
        </Button>
      </div>
      {refusals.map((refusal) => (
        <Callout key={refusal.id} tone="danger">
          {t("knowledge.upload.refused", {
            filename: refusal.filename,
            message: refusal.message,
          })}
        </Callout>
      ))}
    </div>
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
          read by a person at their least patient moment. The hint says to write
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
      {create.isError ? (
        <Callout tone="danger">{problemMessageOf(create.error, t)}</Callout>
      ) : null}
    </PanelBody>
  );
}
