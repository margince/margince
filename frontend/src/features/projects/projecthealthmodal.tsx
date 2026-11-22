import { useEffect, useId, useRef, useState } from "react";
import {
  Button,
  Field,
  Modal,
  SegmentedControl,
  Textarea,
} from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useFieldFaults } from "../../design-system/savepage";
import { trimServerSpace } from "../../format/servertrim";
import { useLocale, useT } from "../../i18n";
import { blankRequired } from "../../lib/problem";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import {
  type ProjectHealthState,
  useCorrectProjectHealth,
  useRecordProjectHealth,
} from "./projecthealth.queries";
import "../../design-system/querystates.css";

const STATES: readonly ProjectHealthState[] = [
  "on_track",
  "at_risk",
  "off_track",
];

/**
 * Recording how the delivery is going, or correcting a reading that was wrong.
 *
 * One modal for both, because they ask the same two questions and differ only
 * in what the answer is filed against. A correction keeps the target's
 * effective time — the server supplies it — so this form never offers to
 * change WHEN something was said, only what was said.
 *
 * The note is required unless the project is on track, and a save without it
 * says so at the note. The server enforces the same rule; stating it in the
 * form is what stops a reader learning it from a refusal.
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
  const { locale } = useLocale();
  const headingId = useId();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
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
  // Each opening starts with no field sentence. Reset during render: `show`
  // is new each render, so an effect listing it would run forever.
  const [faultsOpen, setFaultsOpen] = useState(open);
  if (open !== faultsOpen) {
    setFaultsOpen(open);
    faults.show(new Map());
  }

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

  const trimmed = trimServerSpace(note);
  const noteLabel = t("projectHealth.note");

  async function submit() {
    // The server's own rule, stated here rather than discovered through a 422.
    const blank =
      state === "on_track"
        ? new Map<string, string>()
        : blankRequired(locale, { note: [noteLabel, trimmed] });
    if (faults.show(blank)) {
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
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {correcting
          ? t("projectHealth.correctTitle")
          : t("projectHealth.recordTitle")}
      </Heading>
      <div className="form-stack" ref={faultsRoot}>
        <SegmentedControl
          label={t("projectHealth.stateLabel")}
          options={STATES}
          value={state}
          onChange={(next) => {
            setState(next);
            faults.edited("note");
          }}
          labels={{
            on_track: t("projectHealth.state.onTrack"),
            at_risk: t("projectHealth.state.atRisk"),
            off_track: t("projectHealth.state.offTrack"),
          }}
        />
        <Field
          label={noteLabel}
          required={state !== "on_track"}
          error={faults.of("note")}
          hint={
            state === "on_track"
              ? t("projectHealth.noteOptionalHint")
              : t("projectHealth.noteRequiredHint")
          }
        >
          {(control) => (
            <Textarea
              maxLength={TEXT_LIMITS.note}
              {...control}
              rows={3}
              value={note}
              disabled={write.isPending}
              onChange={(event) => {
                setNote(event.target.value);
                faults.edited("note");
              }}
            />
          )}
        </Field>
        {correcting && (
          <p className="t-caption">{t("projectHealth.correctionNote")}</p>
        )}
        <ErrorLine error={write.error} />
      </div>
      <div className="actions">
        <Button variant="ghost" onClick={onClose} disabled={write.isPending}>
          {t("deals.cancel")}
        </Button>
        <Button onClick={() => void submit()} disabled={write.isPending}>
          {correcting
            ? t("projectHealth.saveCorrection")
            : t("projectHealth.saveReading")}
        </Button>
      </div>
    </Modal>
  );
}
