import { BookOpen, MessageSquare } from "lucide-react";
import type { ReactNode } from "react";
import { Badge, Button } from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import { fileKind } from "../design-system/filechip";
import { Panel, PanelBody } from "../design-system/panel";
import { formatBytes, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import {
  type DealRoomThread,
  ThreadComposer,
  ThreadList,
  type ThreadVerbs,
} from "./dealroomthread";
import "./dealroomthreads.css";

export type { DealRoomThread, ThreadVerbs } from "./dealroomthread";

// The document board, drawn once for both sides. A thread is about one
// document or about the room, and the board keeps each thread under the
// thing it is about: every document is a tile that carries its own threads
// and its own composer, and the room-wide threads sit in one panel below.
// Seller and buyer read the same threads in the same shape (the contract
// serves one projection), so this file holds the rendering and takes the
// verbs as callbacks: who may reply, who may resolve, who may open a thread
// are the caller's decisions, and the only things that differ between the
// two screens.

/** One document as the board draws it; what the sides know differs. */
export type BoardDocument = Readonly<{
  id: string;
  groupKey: string;
  title: string;
  /** The stored filename: what the kind stamp and the saved copy are named by. */
  filename: string;
  /** The filename, group, or both, under the title. */
  meta: string;
  /** The stored size, when the server recorded one. */
  byteSize?: number | null;
  /** The share state the seller sees (the buyer only sees shared ones). */
  status?: ReactNode;
  /** Download or remove — the side's own verbs on the document. */
  actions?: ReactNode;
  /**
   * Opens the document over the page. Absent when this side has no way to
   * draw it: a kind no browser renders, or a surface with no preview mounted.
   */
  read?: () => void;
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
  const { locale } = useLocale();
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
            <p className="t-caption">{empty}</p>
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
        titleAction={<Badge>{formatNumber(roomThreads.length, locale)}</Badge>}
      >
        <PanelBody>
          {roomThreads.length === 0 ? (
            <p className="t-caption">{t("threads.empty")}</p>
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

/**
 * The threads nobody has answered: open, and the last word is the buyer's.
 *
 * One reading on both sides, and it means the same thing on each. To the buyer
 * it is "still waiting on them"; to the seller it is "a buyer is waiting on
 * you" — which is the question a rep opening the room actually has. A thread
 * the seller opened and the buyer has not answered is not counted: the room
 * exists for the buyer's questions, and a seller chasing their own is not
 * what "unanswered" should mean on the buyer's page.
 */
function unansweredCount(threads: readonly DealRoomThread[]): number {
  return threads.filter((thread) => {
    if (thread.state === "resolved") {
      return false;
    }
    const last = thread.comments?.at(-1);
    return last !== undefined && last.author.side === "buyer";
  }).length;
}

// A document tile: the document, then what has been said about it, then the
// place to say more. A reader never has to work out which document a thread
// belongs to, because the thread is inside the document.
//
// A tile rather than a row because a buyer scans a room the way they scan a
// folder — by the paper — and the stage at the top is what makes eight files
// tellable apart before any of them is opened: the kind, the size, and whether
// somebody is waiting on an answer about it.
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
  const plural = usePlural();
  const { locale } = useLocale();
  const unanswered = unansweredCount(threads);
  const kind = fileKind(doc.filename);
  return (
    <article className="board-doc" aria-label={doc.title}>
      <div className="board-doc-stage">
        {/* The page is a shape, not a rendering of the file: the bytes are
            never fetched to draw a thumbnail, and a shape that promised to be
            page one would be a promise the board cannot keep. */}
        <span className="board-doc-page" aria-hidden>
          <span className="board-doc-page-line board-doc-page-head" />
          <span className="board-doc-page-line" />
          <span className="board-doc-page-line board-doc-page-short" />
          <span className="board-doc-page-line" />
          <span className="board-doc-page-line board-doc-page-short" />
        </span>
        {kind ? (
          <span className="board-doc-kind file-chip-kind" aria-hidden>
            {kind}
          </span>
        ) : null}
        {unanswered > 0 ? (
          <span className="board-doc-unanswered">
            <Badge tone="warn">
              {plural("threads.unanswered", unanswered, {
                count: formatNumber(unanswered, locale),
              })}
            </Badge>
          </span>
        ) : null}
      </div>
      <div className="board-doc-body">
        {/* The tile's own door when the file can be read here: the title is a
            button stretched over the stage and the facts by CSS, so pressing
            anywhere on the paper opens it, while the verbs and the threads
            sit above the stretch and keep their own presses. It carries the
            TITLE, not "Read", so a screen reader hears which document it
            opens; the Read verb beside it is the visible spelling of the
            same act. */}
        {doc.read ? (
          <button
            type="button"
            className="board-doc-title board-doc-open"
            aria-haspopup="dialog"
            onClick={doc.read}
          >
            {doc.title}
          </button>
        ) : (
          <p className="board-doc-title">{doc.title}</p>
        )}
        <p className="t-caption board-doc-meta">
          {[
            doc.meta,
            doc.byteSize === null || doc.byteSize === undefined
              ? ""
              : formatBytes(doc.byteSize, locale),
          ]
            .filter((part) => part !== "")
            .join(" · ")}
        </p>
        {doc.status ? (
          <div className="board-doc-status">{doc.status}</div>
        ) : null}
        {doc.read || doc.actions ? (
          <div className="board-doc-verbs">
            {doc.read ? (
              <Button
                variant="primary"
                aria-label={t("threads.readTitle", { title: doc.title })}
                onClick={doc.read}
              >
                <BookOpen aria-hidden />
                {t("threads.read")}
              </Button>
            ) : null}
            {doc.actions}
          </div>
        ) : null}
      </div>
      {threads.length > 0 ? (
        <div className="board-doc-threads">
          <span className="t-caption board-doc-threads-head">
            <MessageSquare size={12} aria-hidden />
            {plural("threads.aboutThis", threads.length, {
              count: formatNumber(threads.length, locale),
            })}
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
