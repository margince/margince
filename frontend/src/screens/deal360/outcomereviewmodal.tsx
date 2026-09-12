import { useEffect, useId, useRef, useState } from "react";
import { Button, Field, Modal, Textarea } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { problemMessageOf } from "../common";
import {
  type ReviewTemplate,
  useCreateOutcomeReview,
} from "../outcomereview.queries";

/**
 * Writing a review of how the deal went.
 *
 * The questions come from the TEMPLATE for the outcome the deal recorded — a
 * win review and a loss review ask different things. They are rendered in the
 * template's own order, and the answers are filed under each question's stable
 * key, so rewording a question later never orphans what somebody wrote.
 */
export function OutcomeReviewModal({
  open,
  onClose,
  dealId,
  closingOccurrenceId,
  template,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  dealId: string;
  // The closing being reviewed. The server refuses a review of any closing but
  // the one the deal is on now, so passing it is how a reopen that happened
  // while this modal sat open comes back as a conflict instead of a review
  // filed against the wrong outcome.
  closingOccurrenceId: string;
  template: ReviewTemplate;
}>) {
  const t = useT();
  const headingId = useId();
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [body, setBody] = useState("");
  // One id per OPENING of the modal, not one per submission attempt. It is what
  // makes a retry idempotent: a save that timed out and was pressed again
  // returns the review that already landed rather than writing a second one.
  // A deliberate second review means opening the modal again, which is when a
  // new id is minted.
  const [submissionId, setSubmissionId] = useState(() => crypto.randomUUID());
  // The id as it is RIGHT NOW, readable from a callback that closed over an
  // older render. State would give submit() the value it captured when the
  // click happened, which is the one question it must not ask.
  const submissionIdRef = useRef(submissionId);
  submissionIdRef.current = submissionId;
  const create = useCreateOutcomeReview(dealId);
  // Pulled out because the effect below depends on THIS function rather than on
  // the mutation object, which is a new object every render: depending on the
  // object would reset the form under the reader mid-typing.
  const resetCreate = create.reset;

  // Re-taken on every open. The modal is mounted for the life of the panel
  // rather than remounted per open, so without this a second review would
  // reuse the first one's submission id and be answered with the first
  // review — silently, looking like a successful save.
  // The closing the draft is ABOUT, captured when the modal opened rather than
  // read at save time.
  //
  // A background refresh can move the deal onto a new closing while this modal
  // sits open — it was reopened and reclosed elsewhere. Sending whatever the
  // prop says by then would file answers written about the old outcome against
  // the new one, and the server would accept them, because the id it was handed
  // is current. Sending the captured one instead makes the server's own
  // stale-closing refusal fire, which is the answer the reader needs: these
  // questions were about a closing that is no longer the one in play.
  const [draftClosing, setDraftClosing] = useState(closingOccurrenceId);

  useEffect(() => {
    if (open) {
      setAnswers({});
      setBody("");
      setSubmissionId(crypto.randomUUID());
      setDraftClosing(closingOccurrenceId);
      resetCreate();
    }
  }, [open, closingOccurrenceId, resetCreate]);

  const missing = template.questions.filter(
    (q) => q.required && !answers[q.key]?.trim(),
  );

  function close() {
    create.reset();
    onClose();
  }

  async function submit() {
    const submitted = submissionId;
    await create.mutateAsync({
      closing_occurrence_id: draftClosing,
      submission_id: submissionId,
      answers,
      body: body.trim() ? body : null,
    });
    // Only close the submission that actually landed. A save over a slow link
    // can return AFTER the reader dismissed the modal and opened it again to
    // write a second review — and an unconditional close there would wipe the
    // new draft on the strength of the old request finishing. The id is minted
    // per opening, so comparing it is exactly the question "is this still the
    // review I was writing".
    if (submitted === submissionIdRef.current) {
      close();
    }
  }

  return (
    <Modal open={open} onClose={close} labelledBy={headingId}>
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {template.label}
      </h2>
      <div className="form-stack">
        {template.questions.map((question) => (
          <Field
            key={question.key}
            label={question.label}
            required={question.required}
          >
            {(control) => (
              <Textarea
                {...control}
                rows={3}
                value={answers[question.key] ?? ""}
                disabled={create.isPending}
                onChange={(event) =>
                  setAnswers((prev) => ({
                    ...prev,
                    [question.key]: event.target.value,
                  }))
                }
              />
            )}
          </Field>
        ))}
        {/* Prose beside the answers, not instead of them. The review is written
            as a note on the timeline, and this is that note's own words. */}
        <Field label={t("outcomeReview.notes")}>
          {(control) => (
            <Textarea
              {...control}
              rows={3}
              value={body}
              disabled={create.isPending}
              onChange={(event) => setBody(event.target.value)}
            />
          )}
        </Field>
        {create.isError && (
          <p className="t-caption" role="alert">
            {problemMessageOf(create.error, t)}
          </p>
        )}
        <div className="actions">
          <Button variant="ghost" onClick={close} disabled={create.isPending}>
            {t("deals.cancel")}
          </Button>
          <Button
            onClick={() => void submit()}
            disabled={create.isPending || missing.length > 0}
          >
            {t("outcomeReview.save")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
