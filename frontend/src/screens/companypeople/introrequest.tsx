import { useMutation } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Modal } from "../../design-system/atoms";
import { ProvenanceTag } from "../../design-system/trust";
import { useT } from "../../i18n";
import { problemMessageOf, throwProblem } from "../common";

// Asking a colleague for the introduction.
//
// The map already tells a reader that Sofia is the warmest way in to Philipp.
// This is the dialog that helps them ask her, and it hands the message BACK
// rather than sending it: there is no colleague-to-colleague channel in this
// product, and the reader sends this under their own name from their own mail
// client.

type Draft = components["schemas"]["AccountEmailDraft"];

export type IntroTarget = Readonly<{
  personId: string;
  personName: string;
  viaUserId: string;
  viaName: string;
}>;

/**
 * IntroRequestModal drafts the ask and lets the reader take it away.
 *
 * The draft is EDITABLE before it is copied. What comes back is a suggestion in
 * the reader's own voice-to-be, not a message the product stands behind — and a
 * dialog that only offered Copy would make every small correction a trip
 * through the clipboard.
 */
export function IntroRequestModal({
  orgId,
  target,
  dealId,
  onClose,
}: Readonly<{
  orgId: string;
  target: IntroTarget | null;
  dealId?: string | null;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  // The reader's edits, held apart from the draft so a re-draft does not
  // silently discard them and an untouched field still follows the model.
  const [edited, setEdited] = useState<{
    subject: string;
    body: string;
  } | null>(null);
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);

  const draft = useMutation({
    mutationFn: async (ask: IntroTarget): Promise<Draft> => {
      const { data, error } = await api.POST(
        "/organizations/{id}/intro-request-draft",
        {
          params: { path: { id: orgId } },
          body: {
            person_id: ask.personId,
            via_user_id: ask.viaUserId,
            ...(dealId ? { deal_id: dealId } : {}),
          },
        },
      );
      if (error || !data) {
        // Through the house helper, so the shared mapper can read the server's
        // own detail and its permission copy. Thrown raw, every refusal —
        // 403, 404, 422 alike — collapsed into "the request failed, no cause
        // reported", which tells a reader nothing about what to do.
        return throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      setEdited(null);
      setCopied(false);
    },
  });

  const written = draft.data;
  const subject = edited?.subject ?? written?.subject ?? "";
  const body = edited?.body ?? written?.body ?? "";
  // A reader who edits owns the words — but ONE KEYSTROKE is not authorship.
  // Flipping the tag on the first character claimed a mostly model-written
  // message as the reader's, which is the same misattribution in the other
  // direction: the mark exists so a reader can tell whose sentences these are.
  const changed =
    edited !== null &&
    (edited.subject !== written?.subject || edited.body !== written?.body);
  const rewritten = changed && editDistanceIsSubstantial(edited, written);

  return (
    <Modal open={target !== null} onClose={onClose} labelledBy={titleId}>
      <h2 id={titleId}>{t("co.intro.title")}</h2>
      {target && (
        <p className="t-caption cp-intro-who">
          {t("co.intro.who", {
            colleague: target.viaName,
            contact: target.personName,
          })}
        </p>
      )}
      {!written && (
        <div className="cp-intro-actions">
          <Button
            variant="ai"
            onClick={() => target && draft.mutate(target)}
            pending={draft.isPending}
            busyLabel={t("co.intro.writing")}
          >
            {t("co.intro.write")}
          </Button>
        </div>
      )}
      {draft.isError && (
        <p className="t-caption cp-intro-error">
          {problemMessageOf(draft.error, t)}
        </p>
      )}
      {written && (
        <>
          <p className="cp-intro-mark">
            <ProvenanceTag
              provenance={
                rewritten
                  ? { kind: "human", self: true }
                  : { kind: "agent", agent: "draft_reply" }
              }
            />
            {written.generated_by === "deterministic" && !rewritten && (
              <span className="t-caption"> {t("co.intro.fromTemplate")}</span>
            )}
          </p>
          <label className="cp-intro-field" htmlFor={`${titleId}-subject`}>
            {t("co.intro.subject")}
          </label>
          <input
            id={`${titleId}-subject`}
            value={subject}
            onChange={(event) => {
              setEdited({ subject: event.target.value, body });
              setCopied(false);
            }}
          />
          <label className="cp-intro-field" htmlFor={`${titleId}-body`}>
            {t("co.intro.body")}
          </label>
          <textarea
            id={`${titleId}-body`}
            rows={10}
            value={body}
            onChange={(event) => {
              setEdited({ subject, body: event.target.value });
              // The clipboard still holds the OLDER text, so a button that
              // went on saying "Copied" would be describing a message the
              // reader can no longer paste.
              setCopied(false);
            }}
          />
          {written.reasoning && written.reasoning.length > 0 && (
            <>
              <p className="t-caption cp-intro-why">{t("co.intro.basedOn")}</p>
              <ul className="chips">
                {written.reasoning.map((reason) => (
                  <li key={`${reason.kind}:${reason.label}`}>{reason.label}</li>
                ))}
              </ul>
            </>
          )}
          {copyFailed && (
            <p className="t-caption cp-intro-error">
              {t("co.intro.copyFailed")}
            </p>
          )}
          <div className="cp-intro-actions">
            {/* Copy first, because it is the one that always works. A mailto:
             * depends on the reader having a mail client bound to the
             * protocol, and a button that silently does nothing is worse than
             * one they did not press. */}
            <Button
              onClick={() => {
                // A browser with no clipboard access, or a refused write, must
                // SAY so. Silently doing nothing leaves a reader pressing a
                // button and wondering why their paste is empty.
                const writer = navigator.clipboard;
                if (!writer) {
                  setCopyFailed(true);
                  return;
                }
                writer
                  .writeText(`${subject}\n\n${body}`)
                  .then(() => {
                    setCopied(true);
                    setCopyFailed(false);
                  })
                  .catch(() => setCopyFailed(true));
              }}
            >
              {copied ? t("co.intro.copied") : t("co.intro.copy")}
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                window.location.href = `mailto:?subject=${encodeURIComponent(
                  subject,
                )}&body=${encodeURIComponent(body)}`;
              }}
            >
              {t("co.intro.openMail")}
            </Button>
          </div>
        </>
      )}
    </Modal>
  );
}

/**
 * editDistanceIsSubstantial reports whether the reader has made the message
 * theirs, rather than fixing a word in it.
 *
 * A tenth of the body is the line, and it is a judgement rather than a
 * measurement: the mark answers "whose sentences are these", and neither
 * answer is free. Flipping on the first keystroke credits a reader with a
 * message the model wrote; never flipping credits the model with one they
 * rewrote. A tenth is where a correction stops being a correction.
 */
function editDistanceIsSubstantial(
  edited: { subject: string; body: string } | null,
  written: Draft | undefined,
): boolean {
  if (!edited || !written) {
    return false;
  }
  if (edited.subject !== written.subject) {
    return true;
  }
  const before = written.body;
  const after = edited.body;
  if (before === after) {
    return false;
  }
  const shifted = Math.abs(after.length - before.length);
  return shifted > Math.max(12, before.length / 10);
}
