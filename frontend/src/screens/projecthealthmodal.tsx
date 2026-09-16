import { useEffect, useId, useRef, useState } from "react";
import {
  Button,
  Field,
  Modal,
  SegmentedControl,
  Textarea,
} from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { RefusalLine } from "./common";
import {
  type ProjectHealthState,
  useCorrectProjectHealth,
  useRecordProjectHealth,
} from "./projecthealth.queries";

const STATES: readonly ProjectHealthState[] = [
  "on_track",
  "at_risk",
  "off_track",
];

/**
 * Trim the way the SERVER trims, so the form and the save agree on what counts
 * as an empty note.
 *
 * Go's `strings.TrimSpace` cuts every Unicode space character. JavaScript's
 * `String.prototype.trim` cuts almost the same set and leaves U+0085 (NEXT
 * LINE) standing. A note holding only that character therefore satisfied this
 * form's "a note is present" check and was refused by the server as missing —
 * the exact failure the check exists to prevent, reached through the one
 * character nobody would think to look for.
 */
function serverTrim(value: string): string {
  return value.replace(/^[\s\u0085]+|[\s\u0085]+$/gu, "");
}

/**
 * Recording how the delivery is going, or correcting a reading that was wrong.
 *
 * One modal for both, because they ask the same two questions and differ only
 * in what the answer is filed against. A correction keeps the target's
 * effective time — the server supplies it — so this form never offers to
 * change WHEN something was said, only what was said.
 *
 * The note is required unless the project is on track, and the save is
 * disabled until it is there. The server enforces the same rule; stating it in
 * the form is what stops a reader learning it from a refusal after they have
 * already written their judgement.
 */
export function ProjectHealthModal({
  open,
  onClose,
  projectId,
  // The reading being corrected, or undefined when recording a new one.
  correcting,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  projectId: string;
  correcting?: Readonly<{
    id: string;
    state: ProjectHealthState;
    note?: string | null;
  }>;
}>) {
  const t = useT();
  const headingId = useId();
  const [state, setState] = useState<ProjectHealthState>("on_track");
  const [note, setNote] = useState("");
  // One id per OPENING. It is what tells a save that landed late apart from
  // the reading the reader is writing now; see submit().
  const [draftId, setDraftId] = useState(() => crypto.randomUUID());
  // The id as it is RIGHT NOW, readable from a callback that closed over an
  // older render. State would give submit() the value it captured when the
  // click happened, which is the one question it must not ask.
  const draftIdRef = useRef(draftId);
  draftIdRef.current = draftId;
  const record = useRecordProjectHealth(projectId);
  const correct = useCorrectProjectHealth(projectId);
  const write = correcting ? correct : record;
  // Pulled out because the effect below depends on THESE functions rather than
  // on the mutation objects, which are new objects every render: depending on
  // the objects would reset the form under the reader mid-edit.
  const resetRecord = record.reset;
  const resetCorrect = correct.reset;

  // Re-seeded on every opening, not once at mount. A correction starts from
  // what the reading actually said, so a reader fixing one word does not have
  // to restate the judgement.
  useEffect(() => {
    if (!open) {
      return;
    }
    setState(correcting?.state ?? "on_track");
    setNote(correcting?.note ?? "");
    setDraftId(crypto.randomUUID());
    resetRecord();
    resetCorrect();
  }, [open, correcting, resetRecord, resetCorrect]);

  const trimmed = serverTrim(note);
  // The server's own rule, stated here rather than discovered through a 422.
  const noteMissing = state !== "on_track" && trimmed === "";

  async function submit() {
    if (noteMissing) {
      return;
    }
    const body = { state, note: trimmed ? trimmed : undefined };
    const submitted = draftId;
    if (correcting) {
      await correct.mutateAsync({ assessmentId: correcting.id, body });
    } else {
      await record.mutateAsync(body);
    }
    // Only close the draft that actually landed. A save over a slow link can
    // return AFTER the reader dismissed this modal and opened it again on
    // another reading, and an unconditional close would wipe the new draft on
    // the strength of the old request finishing.
    if (submitted === draftIdRef.current) {
      onClose();
    }
  }

  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId}>
      <Heading
        size="large"
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {correcting
          ? t("projectHealth.correctTitle")
          : t("projectHealth.recordTitle")}
      </Heading>
      <div className="form-stack">
        <SegmentedControl
          label={t("projectHealth.stateLabel")}
          options={STATES}
          value={state}
          onChange={setState}
          labels={{
            on_track: t("projectHealth.state.onTrack"),
            at_risk: t("projectHealth.state.atRisk"),
            off_track: t("projectHealth.state.offTrack"),
          }}
        />
        <Field
          label={t("projectHealth.note")}
          required={state !== "on_track"}
          hint={
            state === "on_track"
              ? t("projectHealth.noteOptionalHint")
              : t("projectHealth.noteRequiredHint")
          }
        >
          {(control) => (
            <Textarea
              {...control}
              rows={3}
              value={note}
              disabled={write.isPending}
              onChange={(event) => setNote(event.target.value)}
            />
          )}
        </Field>
        {correcting && (
          <p className="t-caption">{t("projectHealth.correctionNote")}</p>
        )}
        {write.isError && <RefusalLine error={write.error} />}
        <div className="actions">
          <Button variant="ghost" onClick={onClose} disabled={write.isPending}>
            {t("deals.cancel")}
          </Button>
          <Button
            onClick={() => void submit()}
            disabled={write.isPending || noteMissing}
          >
            {correcting
              ? t("projectHealth.saveCorrection")
              : t("projectHealth.saveReading")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
