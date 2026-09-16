// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CheckCheck, MessageSquare } from "lucide-react";
import { useState } from "react";
import type { components } from "../api/schema";
import {
  Avatar,
  Badge,
  Button,
  Checkbox,
  Field,
  Textarea,
} from "../design-system/atoms";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { problemMessageOf } from "./common";

// One thread of a Deal Room's conversation, and the composer that opens
// another: what was said, by whom and when, the reply under it, and the verbs
// the caller allows. Drawn the same for both sides — the board in
// dealroomthreads.tsx places these under the document they are about.

// The one element carrying "why you may not write here". The room panel's
// composer prints it; every refused document button points at it with
// `aria-describedby` rather than repeating it. Both are this file's composer,
// and the board mounts the room panel's unconditionally, so the reference
// cannot dangle.
const REFUSAL_ID = "deal-room-write-refusal";

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

export function ThreadList({
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

// One thing said, with who said it and when. The face is the author's own
// monogram, drawn the way every contact in the product is drawn, and the time
// is the reader's — a buyer holds no seat and no workspace zone, and "when did
// they write this" is a question about the reader's clock on either side.
function CommentRow({
  comment,
}: Readonly<{ comment: components["schemas"]["DealRoomComment"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const buyer = comment.author.side === "buyer";
  return (
    <li>
      <Avatar name={comment.author.name} size="xs" />
      <div className="thread-comment">
        <span className="t-caption thread-author">
          <span className="thread-author-name">{comment.author.name}</span>
          {" · "}
          {t(buyer ? "threads.sideBuyer" : "threads.sideSeller")}
          {" · "}
          {formatDateTime(comment.created_at, locale, viewerZone())}
        </span>
        <p>{comment.body}</p>
      </div>
    </li>
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
      setError(problemMessageOf(failure, t));
    } finally {
      setPending(null);
    }
  };
  return (
    <div className="thread">
      {thread.required_change || resolved ? (
        <div className="thread-head">
          {thread.required_change ? (
            <Badge tone="warning">{t("threads.requiredChange")}</Badge>
          ) : null}
          {resolved ? (
            <Badge tone="success">{t("threads.resolved")}</Badge>
          ) : null}
        </div>
      ) : null}
      <ol className="thread-comments">
        {(thread.comments ?? []).map((comment) => (
          <CommentRow key={comment.id} comment={comment} />
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
      {error ? <p className="t-danger">{error}</p> : null}
    </div>
  );
}

// The composer is bound to what it is about: a document's composer opens a
// thread about that document and nothing else, so there is no "this is
// about" picker to get wrong. On a document it starts folded to one button,
// so a card with nothing said yet stays a document and not a form.
export function ThreadComposer({
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
    // A reader who may not write still sees WHERE writing would happen: the
    // button in its disabled state, carrying the reason. Rendering nothing
    // here made a preview show a room with no reply affordance at all, so a
    // rep checking what their buyer sees could not tell a commenting seat
    // from a read-only one — the two drew the same page.
    //
    // The room composer states the sentence and draws its own button; a
    // document card draws the button alone and points at that sentence. Both
    // draw one, because a room with NO documents has no card to carry it —
    // and an empty room shown to a preview is exactly where a rep most needs
    // to see that a buyer would have somewhere to write.
    if (!verbs.refusal) {
      return null;
    }
    return (
      <>
        {collapsible ? null : (
          <p className="t-danger" id={REFUSAL_ID}>
            {verbs.refusal}
          </p>
        )}
        <div className="card-actions">
          {/* `reasonId`, not `reason`: every control on the board is refused
              by the ONE fact the room panel states, and printing that sentence
              under each of them says it as many times as there are files.
              Naming it once and pointing each control at it says it once and
              still reaches a screen reader from every one of them. */}
          <Button variant="ghost" reasonId={REFUSAL_ID}>
            <MessageSquare aria-hidden />
            {label}
          </Button>
        </div>
      </>
    );
  }
  const open = verbs.open;
  if (!openForm) {
    return (
      <div className="card-actions">
        <Button variant="ghost" onClick={() => setOpenForm(true)}>
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
      setError(problemMessageOf(failure, t));
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
          disabled={body.trim() === ""}
          pending={pending}
          onClick={submit}
        >
          {t("threads.open")}
        </Button>
        {collapsible ? (
          <Button variant="ghost" onClick={() => setOpenForm(false)}>
            {t("threads.cancel")}
          </Button>
        ) : null}
      </div>
      {error ? <p className="t-danger">{error}</p> : null}
    </div>
  );
}
