import { useMutation } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field, Modal } from "../../design-system/atoms";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { ProvenanceTag } from "../../design-system/trust";
import { useT } from "../../i18n";
import { throwProblem } from "../common";

// Asking a colleague for the introduction.
//
// The map already tells a reader that Sofia is the warmest way in to Philipp.
// This is the dialog that helps them ask her, and it hands the message BACK
// rather than sending it: there is no colleague-to-colleague channel in this
// product, and the reader sends this under their own name from their own mail
// client.

type Draft = components["schemas"]["CompanyEmailDraft"];

export type IntroTarget = Readonly<{
  contactId: string;
  contactName: string;
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
  companyId,
  target,
  dealId,
  onClose,
}: Readonly<{
  companyId: string;
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

  const draft = useMutation({
    mutationFn: async (ask: IntroTarget): Promise<Draft> => {
      const { data, error } = await api.POST(
        "/companies/{id}/intro-request-draft",
        {
          params: { path: { id: companyId } },
          body: {
            contact_id: ask.contactId,
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
  // Keyed on the message as it stands, so an edit — or a re-draft — stops the
  // button claiming a clipboard that holds the text before it.
  const copy = useClipboardCopy(`${subject}\n\n${body}`, {
    copy: t("co.intro.copy"),
    copied: t("co.intro.copied"),
    remedy: t("co.intro.copyFailed"),
  });

  return (
    <Modal open={target !== null} onClose={onClose} labelledBy={titleId}>
      <Heading size="large" id={titleId}>
        {t("co.intro.title")}
      </Heading>
      {target && (
        <p className="t-caption cp-intro-who">
          {t("co.intro.who", {
            colleague: target.viaName,
            contact: target.contactName,
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
      <ErrorLine error={draft.error} />
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
          <Field className="cp-intro-field" label={t("co.intro.subject")}>
            {(control) => (
              <input
                {...control}
                value={subject}
                onChange={(event) =>
                  setEdited({ subject: event.target.value, body })
                }
              />
            )}
          </Field>
          <Field className="cp-intro-field" label={t("co.intro.body")}>
            {(control) => (
              <textarea
                {...control}
                rows={10}
                value={body}
                onChange={(event) =>
                  setEdited({ subject, body: event.target.value })
                }
              />
            )}
          </Field>
          {written.reasoning && written.reasoning.length > 0 && (
            <>
              <p className="cp-intro-why">{t("co.intro.basedOn")}</p>
              <ul className="chips">
                {written.reasoning.map((reason) => (
                  <li key={`${reason.kind}:${reason.label}`}>{reason.label}</li>
                ))}
              </ul>
            </>
          )}
          {copy.notice}
          <div className="cp-intro-actions">
            {/* Copy first, because it is the one that always works. A mailto:
             * depends on the reader having a mail client bound to the
             * protocol, and a button that silently does nothing is worse than
             * one they did not press. */}
            <Button onClick={copy.copy}>{copy.label}</Button>
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
