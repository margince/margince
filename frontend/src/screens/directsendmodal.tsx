// Sending a message the engine refused, on a named human's own authority.
//
// This is the last thing somebody sees before an exception is recorded under
// their name. Everything about it is shaped by that: the warning is the
// server's words rather than ours, the acknowledgement starts unticked, and the
// reason is typed rather than chosen from a list nobody would read.
//
// THE REFUSAL IS NOT BEING CORRECTED. Confirming does not grant consent and
// does not lift a stop — the engine's answer stays exactly where it is, and the
// record says a human overrode it. The copy says so because a director who
// believed otherwise would be acknowledging something untrue.
//
// WHY THE WARNING IS NOT OUR OWN COPY. The instruction records which version of
// the caution was acknowledged. If this file wrote the words, the record would
// name a version whose text lives in a client somebody can ship separately from
// the server — and a dispute about the override could not be answered with the
// sentence the director actually read.

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Field } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { throwProblem } from "./common";

// The review as this modal needs it: which message, and the caution the server
// serves for it. Taken from the generated contract rather than restated, so a
// field the server renames cannot go on being read here.
type CommunicationReview = components["schemas"]["CommunicationReview"];

// DirectSendCommand is what the server is told. The whole of it travels as a
// variable rather than closed over render state: a mutation reading the reason
// from the component's scope would send whatever was typed when it resolved
// rather than what was there when the director pressed confirm.
type DirectSendCommand = Readonly<{
  reviewId: string;
  reasonCode: string;
  explanation: string;
  warningVersion: string;
}>;

async function directSend(command: DirectSendCommand): Promise<string> {
  const { data, error, response } = await api.POST(
    "/communication-reviews/{id}/direct-send",
    {
      params: { path: { id: command.reviewId } },
      body: {
        reason_code: command.reasonCode as "other",
        explanation: command.explanation,
        warning_version: command.warningVersion,
        // The tick IS the acknowledgement, and the server refuses a decision
        // without one. It is sent as true because this function is only
        // reachable from a confirm the director could not press until they
        // ticked it — never defaulted, which would record a decision nobody
        // made.
        acknowledged: true,
      },
    },
  );
  // Success is a real 2xx WITH a body: openapi-fetch reports a falsy error and
  // undefined data for a bodiless non-2xx, which would otherwise read as a
  // message that went when nothing did.
  if (!response.ok || !data) {
    throwProblem(error || { title: "The message was not sent." });
  }
  return data.id;
}

// REASON_CODES is the closed list the record keeps, in the server's own
// vocabulary. Closed because an audit of overrides has to be countable: free
// text alone cannot answer "how often do we send on a contract clause".
const REASON_CODES = [
  "customer_requested_outside_crm",
  "contractual_necessity",
  "legal_obligation",
  "other",
] as const;

type ReasonCode = (typeof REASON_CODES)[number];

export function DirectSendModal({
  open,
  onClose,
  review,
  onSent,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  review: CommunicationReview;
  onSent?: (activityId: string) => void;
}>) {
  const t = useT();
  const [acknowledged, setAcknowledged] = useState(false);
  const [reasonCode, setReasonCode] = useState<ReasonCode>("other");
  const [explanation, setExplanation] = useState("");

  // RESET WHEN THE MESSAGE CHANGES. A director who ticks, cancels, and opens a
  // DIFFERENT refused message would otherwise arrive with the acknowledgement
  // already ticked and the previous reason still typed — one click from
  // recording, against this message, a decision they made about another one.
  //
  // Keyed on the review rather than on `open`, because reopening the same
  // message after a cancel is the one case where keeping what they typed is
  // the kindness rather than the hazard.
  const [seen, setSeen] = useState(review.id);
  if (seen !== review.id) {
    setSeen(review.id);
    setAcknowledged(false);
    setReasonCode("other");
    setExplanation("");
  }

  const send = useMutation({
    mutationFn: directSend,
    onSuccess: (activityId) => {
      onSent?.(activityId);
      onClose();
    },
  });

  const said = explanation.trim();
  // BOTH, and the modal says which is missing rather than refusing silently.
  // The explanation is what a dispute reads; the tick is the act.
  // AND THE WARNING ITSELF. An installation whose server serves none — one
  // older than this client — leaves nothing for the director to acknowledge,
  // and submitting an empty version would be refused by the server with an
  // error about a field the director never saw. Saying so here is the honest
  // answer: there is nothing to read, so there is nothing to acknowledge.
  const missing = !review.warning
    ? t("directSend.noWarningServed")
    : !acknowledged
      ? t("directSend.needsAcknowledgement")
      : said === ""
        ? t("directSend.needsReason")
        : undefined;

  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      size="wide"
      title={t("directSend.title")}
      confirmLabel={t("directSend.confirm")}
      confirmVariant="danger"
      confirmDisabled={Boolean(missing)}
      confirmReason={missing}
      pending={send.isPending}
      error={send.isError ? t("directSend.failed") : undefined}
      onConfirm={() =>
        send.mutate({
          reviewId: review.id,
          reasonCode,
          explanation: said,
          warningVersion: review.warning?.version ?? "",
        })
      }
    >
      {/* THE SERVER'S OWN WORDS, verbatim. Paraphrasing here would record the
          director as having read the server's caution while showing them
          ours. */}
      {review.warning && (
        <p
          className="t-body"
          role="alert"
          style={{ color: "var(--dangerText)" }}
        >
          {review.warning.text}
        </p>
      )}
      <Field label={t("directSend.reasonCodeLabel")}>
        {(control) => (
          <Select
            {...control}
            value={reasonCode}
            onChange={(picked) => setReasonCode(picked as ReasonCode)}
            options={REASON_CODES.map((code) => ({
              value: code,
              label: t(
                `directSend.reason.${code}` as "directSend.reason.other",
              ),
            }))}
          />
        )}
      </Field>
      {/* A TEXTAREA so Enter inserts a newline. A single-line input here would
          submit the form on Enter, which is the one key a human typing a
          reason presses without meaning to confirm anything. */}
      <Field label={t("directSend.explanationLabel")}>
        {(control) => (
          <textarea
            {...control}
            className="input"
            rows={3}
            maxLength={1000}
            value={explanation}
            onChange={(e) => setExplanation(e.target.value)}
          />
        )}
      </Field>
      {/* UNTICKED, always. A pre-ticked acknowledgement records a decision
          nobody made, which is the one thing this record must never say. */}
      <label className="t-body">
        <input
          type="checkbox"
          checked={acknowledged}
          onChange={(e) => setAcknowledged(e.target.checked)}
        />{" "}
        {t("directSend.acknowledge")}
      </label>
    </ConfirmModal>
  );
}
