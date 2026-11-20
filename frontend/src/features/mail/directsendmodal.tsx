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
import { api } from "../../api/client";
import { intentKeys } from "../../api/idempotencykey";
import type { components } from "../../api/schema";
import { CitedLaws, FindingWithLaw } from "../../components/citedlaw";
import { lawsOf } from "../../components/citedprovisions";
import { Button, Checkbox, Field, Textarea } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Select } from "../../design-system/select";
import { footerFindingsText } from "../../format/footerfindings";
import { sendingCompanyScope } from "../../format/sendingcompany";
import { sendPermissionReasonKey } from "../../format/sendpermissionreason";
import { useLocale, useT } from "../../i18n";
import { communicationContextLabel } from "../../lib/compose-context";
import { throwProblem } from "../../lib/problem";
import type { ScheduleFields } from "./composeschedule";
import { DirectSendWhen, useSendAnywayWhen } from "./directsendwhen";
import { refusalCountryText } from "./refusalcountry";
import { sendReviewOf } from "./sendreview";
import { WordingCorrectionModal } from "./wordingcorrectionmodal";

// The review as this modal needs it: which message, and the caution the server
// serves for it. Taken from the generated contract rather than restated, so a
// field the server renames cannot go on being read here.
type CommunicationReview = components["schemas"]["CommunicationReview"];
type RefusedRecipient = CommunicationReview["refusals"][number];

// DirectSendCommand is what the server is told. The whole of it travels as a
// variable rather than closed over render state: a mutation reading the reason
// from the component's scope would send whatever was typed when it resolved
// rather than what was there when the director pressed confirm.
export type DirectSendCommand = Readonly<{
  reviewId: string;
  reasonCode: string;
  explanation: string;
  warningVersion: string;
  assessmentRevision?: string;
  // The refusals the director read. A review rewritten since then is stale.
  shownReasons: readonly string[];
  // A later moment to send at. Absent sends now.
  later?: ScheduleFields;
}>;

/** What a Send anyway came to: sent now, waiting for its moment, or gone
 *  already from another tab. */
export type DirectSendOutcome = "sent" | "scheduled" | "alreadySent";

// A decision retried after a lost answer reuses its key, whichever reviews
// were decided in between. The server then replays the send it recorded
// instead of answering that the review is gone.
const decisionKeys = intentKeys();

export async function directSend(
  command: DirectSendCommand,
): Promise<DirectSendOutcome> {
  const body = {
    reason_code: command.reasonCode as "other",
    explanation: command.explanation,
    warning_version: command.warningVersion,
    assessment_revision: command.assessmentRevision,
    shown_reasons: [...command.shownReasons],
    // The server refuses a decision without an acknowledgement. Each
    // caller reaches this only from the press that is one. That is the
    // modal's confirm once ticked, or Send anyway beside its warning.
    acknowledged: true as const,
    ...command.later,
  };
  const { data, error, response } = await decisionKeys(
    [command.reviewId, body],
    (key) =>
      api.POST("/communication-reviews/{id}/direct-send", {
        params: {
          path: { id: command.reviewId },
          header: { "Idempotency-Key": key },
        },
        body,
      }),
  );
  // Success is a real 2xx WITH a body: openapi-fetch reports a falsy error and
  // undefined data for a bodiless non-2xx, which would otherwise read as a
  // message that went when nothing did.
  if (!response.ok || !data) {
    if (response.status === 409 && error?.code === "already_sent") {
      return "alreadySent";
    }
    throwProblem(error || { title: "The message was not sent." });
  }
  return response.status === 202 ? "scheduled" : "sent";
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
  review: initialReview,
  onSent,
  onReviewRefreshed,
  offerLater,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  review: CommunicationReview;
  onSent?: (outcome: DirectSendOutcome) => void;
  onReviewRefreshed?: (review: CommunicationReview) => void;
  /** Offer a later moment, for a message held in the scheduled list. */
  offerLater?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const when = useSendAnywayWhen();
  const [review, setReview] = useState(initialReview);
  const correction = review.wording_correction;
  const prior = correction ?? {
    reason_code: "other",
    explanation: "",
    version: "new",
  };
  const recordedReason =
    REASON_CODES.find((code) => code === prior.reason_code) ?? "other";
  const [editingWording, setEditingWording] =
    useState<CommunicationReview["wording_correction"]>();
  const [acknowledged, setAcknowledged] = useState(false);
  const [reasonCode, setReasonCode] = useState<ReasonCode>(recordedReason);
  const [explanation, setExplanation] = useState(prior.explanation);
  const sourceKey = reviewKey(initialReview);
  const [source, setSource] = useState({
    message: initialReview.delivery_intent_id ?? initialReview.id,
    key: sourceKey,
  });
  const [refreshFailed, setRefreshFailed] = useState(false);
  const [assessmentChanged, setAssessmentChanged] = useState(false);
  if (source.key !== sourceKey) {
    const message = initialReview.delivery_intent_id ?? initialReview.id;
    setSource({ message, key: sourceKey });
    setReview(initialReview);
    if (source.message !== message)
      setExplanation(initialReview.wording_correction?.explanation ?? "");
    setAcknowledged(false);
  }

  // A different review or warning needs a fresh acknowledgement.
  const acknowledgementKey = `${reviewKey(review)}:${prior.version}`;
  const [seen, setSeen] = useState(acknowledgementKey);
  if (seen !== acknowledgementKey) {
    setSeen(acknowledgementKey);
    setAcknowledged(false);
    setEditingWording(undefined);
  }

  const send = useMutation({
    mutationFn: directSend,
    onMutate: () => setAssessmentChanged(false),
    onError: async (error) => {
      const refreshed = sendReviewOf(error);
      if (!refreshed?.reviewId || refreshed.reviewId === review.id) return;
      try {
        const response = await api.GET("/communication-reviews/{id}", {
          params: { path: { id: refreshed.reviewId } },
        });
        if (response.data) {
          setReview(response.data);
          onReviewRefreshed?.(response.data);
          setAcknowledged(false);
          setRefreshFailed(false);
          setAssessmentChanged(true);
        } else {
          setRefreshFailed(true);
        }
      } catch {
        setRefreshFailed(true);
      }
    },
    onSuccess: (outcome) => {
      onSent?.(outcome);
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
  const missing =
    confirmationMissing(Boolean(review.warning), acknowledged, said, t) ??
    when.missing;

  if (editingWording) {
    return (
      <WordingCorrectionModal
        reviewId={review.id}
        context={editingWording}
        onClose={() => setEditingWording(undefined)}
        onSaved={onClose}
      />
    );
  }

  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      intent="form"
      title={t("directSend.title")}
      confirmLabel={when.confirmLabel}
      confirmVariant="danger"
      confirmDisabled={Boolean(missing)}
      confirmReason={missing}
      pending={send.isPending}
      error={confirmationError(
        assessmentChanged,
        refreshFailed,
        send.isError,
        t,
      )}
      onConfirm={() =>
        send.mutate({
          reviewId: review.id,
          reasonCode,
          explanation: said,
          warningVersion: review.warning?.version ?? "",
          assessmentRevision: review.assessment_revision,
          shownReasons: (review.refusals ?? []).map(
            (refusal) => refusal.reason_code,
          ),
          later: when.chosen.later,
        })
      }
    >
      {/* THE SERVER'S OWN WORDS, verbatim. Paraphrasing here would record the
          director as having read the server's caution while showing them
          ours. */}
      {review.warning && <p className="t-body">{review.warning.text}</p>}
      {correction && (
        <>
          <p className="t-body">{t("directSend.existingApproval")}</p>
          <Button variant="ghost" onClick={() => setEditingWording(correction)}>
            {t("directSend.correctWording")}
          </Button>
        </>
      )}
      {review.refusals.map((refusal) => (
        <div
          key={`${refusal.address}-${refusal.subject_id ?? ""}-${refusal.category ?? ""}`}
        >
          <p>{refusal.address}</p>
          {refusalCountryText(refusal, t, locale) && (
            <p className="t-caption">
              {refusalCountryText(refusal, t, locale)}
            </p>
          )}
          <ConfirmationFooter refusal={refusal} />
          <p className="t-caption">
            <ReasonWithLaw refusal={refusal} />
          </p>
          <p className="t-caption">
            {communicationContextLabel(
              refusal.requested_category ?? refusal.category,
              t,
            ) ?? t("consent.exceptionOtherPurpose")}
          </p>
          <p className="t-caption">
            {sendingCompanyScope(
              refusal.sending_entity_id,
              refusal.sending_entity_name,
              t,
            )}
          </p>
        </div>
      ))}
      <Field label={t("directSend.reasonCodeLabel")}>
        {(control) => (
          <Select
            {...control}
            value={reasonCode}
            disabled={Boolean(correction)}
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
          <Textarea
            {...control}
            rows={3}
            maxLength={1000}
            value={explanation}
            disabled={Boolean(correction)}
            onChange={(e) => setExplanation(e.target.value)}
          />
        )}
      </Field>
      {/* UNTICKED, always. A pre-ticked acknowledgement records a decision
          nobody made, which is the one thing this record must never say. */}
      <Checkbox
        className="t-body"
        checked={acknowledged}
        label={t("directSend.acknowledge")}
        onChange={(e) => setAcknowledged(e.target.checked)}
      />
      {offerLater && <DirectSendWhen state={when} />}
    </ConfirmModal>
  );
}

function ConfirmationFooter({
  refusal,
}: Readonly<{ refusal: CommunicationReview["refusals"][number] }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      {refusal.footer_preview?.length ? (
        <div>
          <p>{t("sendPermission.footer.preview")}</p>
          {refusal.footer_preview.map((text) => (
            <p key={text} className="t-caption">
              {text}
            </p>
          ))}
        </div>
      ) : null}
      {refusal.footer_findings?.length ? (
        <p>
          {footerFindingsText(refusal.footer_findings, t, locale)}{" "}
          <CitedLaws provisions={refusal.footer_findings.flatMap(lawsOf)} />
        </p>
      ) : null}
    </>
  );
}

function confirmationError(
  changed: boolean,
  refreshFailed: boolean,
  failed: boolean,
  t: ReturnType<typeof useT>,
) {
  if (changed) return t("directSend.changed");
  if (refreshFailed) return t("directSend.couldNotOpen");
  return failed ? t("directSend.failed") : undefined;
}

function reviewKey(review: CommunicationReview): string {
  return `${review.id}:${review.warning?.version ?? ""}:${JSON.stringify(review.refusals)}`;
}

function confirmationMissing(
  hasWarning: boolean,
  acknowledged: boolean,
  explanation: string,
  t: ReturnType<typeof useT>,
) {
  if (!hasWarning) return t("directSend.noWarningServed");
  if (!acknowledged) return t("directSend.needsAcknowledgement");
  if (!explanation) return t("directSend.needsReason");
  return undefined;
}

// Why the engine refused this recipient, with the laws it rests on where a pack
// binding the message cites one.
function ReasonWithLaw({ refusal }: Readonly<{ refusal: RefusedRecipient }>) {
  const t = useT();
  const sentence = t(
    sendPermissionReasonKey(refusal.reason_code, {}, refusal.category),
  );
  return <FindingWithLaw sentence={sentence} provisions={lawsOf(refusal)} />;
}
