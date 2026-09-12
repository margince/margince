// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The colleague's side: what is being asked of them, and the four answers.
//
// This is a surface rather than a row with buttons on it, because the answer is
// a judgement about a relationship the colleague owns. They read who is asking,
// why, what the contact gets, and the words they would be forwarding under
// their own name — and only then do they answer.

import { useId, useState } from "react";
import { Badge, Button, Field, Modal } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { useT } from "../i18n";
import {
  type IntroDecision,
  type IntroRequest,
  useDecideIntroRequest,
} from "./introrequests";

/**
 * IntroDecisionDrawer answers one ask.
 *
 * `name_drop_approved` sits beside `accepted` as an equal rather than under it:
 * lending your name and making the introduction are different favours, and a
 * surface that framed one as a lesser form of the other would push a colleague
 * into the wrong one.
 */
export function IntroDecisionDrawer({
  contactId,
  contactName,
  request,
  open,
  onClose,
}: Readonly<{
  contactId: string;
  contactName: string;
  request: IntroRequest;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const [answer, setAnswer] = useState<IntroDecision>("accepted");
  const [reason, setReason] = useState("");

  const decide = useDecideIntroRequest(contactId);

  // Suggesting somebody else without naming them is not an answer, and the
  // server refuses it. Until a colleague picker exists this surface offers the
  // other three, so it cannot present a choice that always fails.
  const answers: readonly IntroDecision[] = [
    "accepted",
    "name_drop_approved",
    "declined",
  ];

  const submit = () => {
    decide.mutate(
      {
        id: request.id,
        decision: answer,
        // The version this surface READ, so a colleague answering in two tabs
        // is told the ask moved rather than overwriting the answer they gave
        // in the other one.
        version: request.version,
        ...(reason.trim() ? { reason: reason.trim() } : {}),
      },
      { onSuccess: () => onClose() },
    );
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      placement="right"
      size="wide"
    >
      <h2 id={titleId}>
        {t("contact.intro.decideTitle", { name: contactName })}
      </h2>

      <p>{request.internal_reason}</p>
      {request.value_for_target ? (
        <p>
          <strong>{t("contact.intro.valueLabel")}: </strong>
          {request.value_for_target}
        </p>
      ) : null}

      {request.forwardable_note ? (
        <section>
          <h3>{t("contact.intro.noteLabel")}</h3>
          {/* Provenance beside the words, not in a tooltip: the colleague is
              about to forward these under their own name, so whether a model
              wrote them is part of what they are deciding. */}
          {request.note_ai_generated ? (
            <Badge tone="ai" quiet>
              {t("contact.intro.noteByModel")}
            </Badge>
          ) : null}
          <blockquote>{request.forwardable_note}</blockquote>
        </section>
      ) : null}

      {request.name_drop_allowed ? (
        <p>{t("contact.intro.nameDropRequested")}</p>
      ) : null}

      <ChoiceList<IntroDecision>
        legend={t("contact.intro.decideLegend")}
        value={answer}
        onChange={setAnswer}
        choices={answers.map((a) => ({
          value: a,
          label: t(DECISION_LABEL[a]),
          description: t(DECISION_HELP[a]),
        }))}
      />

      <Field
        label={t("contact.intro.decideReasonLabel")}
        hint={t("contact.intro.decideReasonHint")}
      >
        {(control) => (
          <textarea
            {...control}
            rows={3}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        )}
      </Field>

      <div className="form-actions">
        <Button onClick={onClose} variant="ghost">
          {t("contact.intro.cancel")}
        </Button>
        <Button onClick={submit} disabled={decide.isPending}>
          {t("contact.intro.decideAction")}
        </Button>
      </div>
      {decide.isError ? (
        <p role="alert">
          <Badge tone="danger">{t("contact.intro.decideFailed")}</Badge>
        </p>
      ) : null}
    </Modal>
  );
}

// The two maps are total over the answers this surface offers, so an answer
// added without its words fails to compile rather than rendering a key.
const DECISION_LABEL: Record<
  IntroDecision,
  Parameters<ReturnType<typeof useT>>[0]
> = {
  accepted: "contact.intro.answerAccept",
  name_drop_approved: "contact.intro.answerNameDrop",
  suggest_other: "contact.intro.answerSuggest",
  declined: "contact.intro.answerDecline",
};

const DECISION_HELP: Record<
  IntroDecision,
  Parameters<ReturnType<typeof useT>>[0]
> = {
  accepted: "contact.intro.answerAcceptHelp",
  name_drop_approved: "contact.intro.answerNameDropHelp",
  suggest_other: "contact.intro.answerSuggestHelp",
  declined: "contact.intro.answerDeclineHelp",
};
