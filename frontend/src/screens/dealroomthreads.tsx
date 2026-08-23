import { CheckCheck, MessageSquare } from "lucide-react";
import { type ReactNode, useState } from "react";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  Textarea,
} from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import "./dealroomthreads.css";

// The document board, drawn once for both sides. A thread is about one
// document or about the room, and the board keeps each thread under the
// thing it is about: every document is a card that carries its own threads
// and its own composer, and the room-wide threads sit in one panel below.
// Seller and buyer read the same threads in the same shape (the contract
// serves one projection), so this file holds the rendering and takes the
// verbs as callbacks: who may reply, who may resolve, who may open a thread
// are the caller's decisions, and the only things that differ between the
// two screens.

export type DealRoomThread = components["schemas"]["DealRoomThread"];

export type ThreadVerbs = Readonly<{
  /** Posts a reply; absent when this reader may not write. */
  reply?: (threadId: string, body: string) => Promise<unknown>;
  /** Resolves a thread; absent when this reader may not (the buyer never may). */
  resolve?: (threadId: string) => Promise<unknown>;
  /** Opens a thread; absent when this reader may not write. */
  open?: (input: {
    documentId: string | null;
    body: string;
    requiredChange: boolean;
  }) => Promise<unknown>;
  /** Whether the composer offers "requires a change" (the buyer's mark). */
  mayRequireChange: boolean;
  /** A sentence saying why writing is refused, when it is. */
  refusal?: string;
}>;

/** One document as the board draws it; what the sides know differs. */
export type BoardDocument = Readonly<{
  id: string;
  groupKey: string;
  title: string;
  /** The filename, group, or both, under the title. */
  meta: string;
  /** The share state the seller sees (the buyer only sees shared ones). */
  status?: ReactNode;
  /** Download or remove — the side's own verbs on the document. */
  actions?: ReactNode;
}>;

export type BoardGroup = Readonly<{ key: string; label: string }>;

export function DocumentBoard({
  title,
  sub,
  titleAction,
  groups,
  documents,
  threads,
  verbs,
  empty,
  footer,
}: Readonly<{
  title: string;
  sub: string;
  titleAction?: ReactNode;
  groups: readonly BoardGroup[];
  documents: readonly BoardDocument[];
  threads: readonly DealRoomThread[];
  verbs: ThreadVerbs;
  empty: string;
  /** Drawn after the documents: the seller's "add a document" form. */
  footer?: ReactNode;
}>) {
  const t = useT();
  const byDocument = new Map<string, DealRoomThread[]>();
  for (const thread of threads) {
    if (thread.document_id) {
      const list = byDocument.get(thread.document_id) ?? [];
      list.push(thread);
      byDocument.set(thread.document_id, list);
    }
  }
  // A thread about a document the list no longer carries still has to be
  // readable and answerable: a seller who removed a document before publishing
  // it can otherwise no longer see, answer or resolve the question the buyer
  // asked about it, while the buyer — still on the last release — can. It joins
  // the room-wide panel rather than disappearing.
  const shown = new Set(documents.map((doc) => doc.id));
  const roomThreads = threads.filter(
    (thread) => !thread.document_id || !shown.has(thread.document_id),
  );
  return (
    <>
      <Panel title={title} sub={sub} titleAction={titleAction}>
        {documents.length === 0 ? (
          <PanelBody>
            <p className="t-small">{empty}</p>
          </PanelBody>
        ) : (
          groups.map((group) => {
            const inGroup = documents.filter((d) => d.groupKey === group.key);
            if (inGroup.length === 0) {
              return null;
            }
            return (
              <PanelBody key={group.key}>
                <Eyebrow as="h3">{group.label}</Eyebrow>
                <div className="board-group">
                  {inGroup.map((doc) => (
                    <DocumentCard
                      key={doc.id}
                      doc={doc}
                      threads={byDocument.get(doc.id) ?? []}
                      verbs={verbs}
                    />
                  ))}
                </div>
              </PanelBody>
            );
          })
        )}
        {footer ? <PanelBody>{footer}</PanelBody> : null}
      </Panel>
      <Panel
        title={t("threads.roomTitle")}
        sub={t("threads.roomSub")}
        titleAction={<Badge>{String(roomThreads.length)}</Badge>}
      >
        <PanelBody>
          {roomThreads.length === 0 ? (
            <p className="t-small">{t("threads.empty")}</p>
          ) : null}
          <ThreadList threads={roomThreads} verbs={verbs} />
          <ThreadComposer
            verbs={verbs}
            documentId={null}
            label={t("threads.newLabel")}
          />
        </PanelBody>
      </Panel>
    </>
  );
}

// A document card: the document, then what has been said about it, then the
// place to say more. A reader never has to work out which document a thread
// belongs to, because the thread is inside the document.
function DocumentCard({
  doc,
  threads,
  verbs,
}: Readonly<{
  doc: BoardDocument;
  threads: readonly DealRoomThread[];
  verbs: ThreadVerbs;
}>) {
  const t = useT();
  return (
    <article className="board-doc" aria-label={doc.title}>
      <div className="room-doc">
        <div>
          <p className="board-doc-title">{doc.title}</p>
          <p className="t-small">{doc.meta}</p>
          {doc.status ? (
            <div className="board-doc-status">{doc.status}</div>
          ) : null}
        </div>
        {doc.actions ? <div className="card-actions">{doc.actions}</div> : null}
      </div>
      {threads.length > 0 ? (
        <div className="board-doc-threads">
          <span className="t-small board-doc-threads-head">
            <MessageSquare aria-hidden />
            {threads.length === 1
              ? t("threads.aboutThisOne")
              : t("threads.aboutThis", { count: String(threads.length) })}
          </span>
          <ThreadList threads={threads} verbs={verbs} />
        </div>
      ) : null}
      <ThreadComposer
        verbs={verbs}
        documentId={doc.id}
        label={t("threads.askAbout")}
        collapsible
      />
    </article>
  );
}

function ThreadList({
  threads,
  verbs,
}: Readonly<{ threads: readonly DealRoomThread[]; verbs: ThreadVerbs }>) {
  return (
    <div className="board-threads">
      {threads.map((thread) => (
        <ThreadRow key={thread.id} thread={thread} verbs={verbs} />
      ))}
    </div>
  );
}

function ThreadRow({
  thread,
  verbs,
}: Readonly<{ thread: DealRoomThread; verbs: ThreadVerbs }>) {
  const t = useT();
  const [reply, setReply] = useState("");
  const [pending, setPending] = useState<"reply" | "resolve" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const resolved = thread.state === "resolved";
  const act = async (
    kind: "reply" | "resolve",
    run: () => Promise<unknown>,
  ) => {
    setPending(kind);
    setError(null);
    try {
      await run();
      if (kind === "reply") {
        setReply("");
      }
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setPending(null);
    }
  };
  return (
    <div className="thread">
      {thread.required_change || resolved ? (
        <div className="thread-head">
          {thread.required_change ? (
            <Badge tone="warn">{t("threads.requiredChange")}</Badge>
          ) : null}
          {resolved ? (
            <Badge tone="success">{t("threads.resolved")}</Badge>
          ) : null}
        </div>
      ) : null}
      <ol className="thread-comments">
        {(thread.comments ?? []).map((comment) => (
          <li
            key={comment.id}
            className={
              comment.author.side === "buyer" ? "thread-buyer" : "thread-seller"
            }
          >
            <span className="t-small thread-author">
              {comment.author.name} ·{" "}
              {t(
                comment.author.side === "buyer"
                  ? "threads.sideBuyer"
                  : "threads.sideSeller",
              )}
            </span>
            <p>{comment.body}</p>
          </li>
        ))}
      </ol>
      {!resolved && verbs.reply ? (
        <div className="thread-reply">
          <Field label={t("threads.replyLabel")}>
            {(control) => (
              <Textarea
                {...control}
                rows={2}
                value={reply}
                onChange={(event) => setReply(event.target.value)}
              />
            )}
          </Field>
          <div className="card-actions">
            <Button
              small
              disabled={reply.trim() === ""}
              pending={pending === "reply"}
              onClick={() => {
                const run = verbs.reply;
                if (run) {
                  act("reply", () => run(thread.id, reply.trim()));
                }
              }}
            >
              {t("threads.reply")}
            </Button>
            {verbs.resolve ? (
              <Button
                small
                variant="ghost"
                pending={pending === "resolve"}
                onClick={() => {
                  const run = verbs.resolve;
                  if (run) {
                    act("resolve", () => run(thread.id));
                  }
                }}
              >
                <CheckCheck aria-hidden />
                {t("threads.resolve")}
              </Button>
            ) : null}
          </div>
        </div>
      ) : null}
      {error ? <p className="t-small t-danger">{error}</p> : null}
    </div>
  );
}

// The composer is bound to what it is about: a document's composer opens a
// thread about that document and nothing else, so there is no "this is
// about" picker to get wrong. On a document it starts folded to one button,
// so a card with nothing said yet stays a document and not a form.
function ThreadComposer({
  verbs,
  documentId,
  label,
  collapsible = false,
}: Readonly<{
  verbs: ThreadVerbs;
  documentId: string | null;
  label: string;
  collapsible?: boolean;
}>) {
  const t = useT();
  const [openForm, setOpenForm] = useState(!collapsible);
  const [body, setBody] = useState("");
  const [requiredChange, setRequiredChange] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (!verbs.open) {
    // The refusal is said once, on the room panel, not under every document.
    return !collapsible && verbs.refusal ? (
      <p className="t-small">{verbs.refusal}</p>
    ) : null;
  }
  const open = verbs.open;
  if (!openForm) {
    return (
      <div className="card-actions">
        <Button small variant="ghost" onClick={() => setOpenForm(true)}>
          <MessageSquare aria-hidden />
          {label}
        </Button>
      </div>
    );
  }
  const submit = async () => {
    setPending(true);
    setError(null);
    try {
      await open({
        documentId,
        body: body.trim(),
        requiredChange: documentId !== null && requiredChange,
      });
      setBody("");
      setRequiredChange(false);
      if (collapsible) {
        setOpenForm(false);
      }
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setPending(false);
    }
  };
  return (
    <div className="thread-composer">
      <Field label={label}>
        {(control) => (
          <Textarea
            {...control}
            rows={3}
            value={body}
            onChange={(event) => setBody(event.target.value)}
          />
        )}
      </Field>
      {verbs.mayRequireChange && documentId !== null ? (
        <Checkbox
          label={t("threads.requireChangeLabel")}
          checked={requiredChange}
          onChange={(event) => setRequiredChange(event.target.checked)}
        />
      ) : null}
      <div className="card-actions">
        <Button
          small
          disabled={body.trim() === ""}
          pending={pending}
          onClick={submit}
        >
          {t("threads.open")}
        </Button>
        {collapsible ? (
          <Button small variant="ghost" onClick={() => setOpenForm(false)}>
            {t("threads.cancel")}
          </Button>
        ) : null}
      </div>
      {error ? <p className="t-small t-danger">{error}</p> : null}
    </div>
  );
}
