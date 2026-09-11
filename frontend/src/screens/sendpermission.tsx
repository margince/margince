import { ShieldAlert, ShieldQuestion } from "lucide-react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CommunicationStatus } from "../design-system/communicationstatus";
import { useT } from "../i18n";

// What the engine decided about this message, said where the rep is writing it.
//
// Until now a rep learned a send was refused by pressing Send and reading an
// error. The engine had already worked the answer out — the preview endpoint
// has answered since the authorization engine landed — and nothing asked it.
//
// THREE STATES, and a rep sees exactly one. Which one is chosen by the engine's
// answer, never by asking the rep to classify their own message: they already
// said what they are writing by picking a person and a thread.
//
//   1. allowed      one quiet line, no interaction. The overwhelming majority
//                   of sends, and it should cost no attention at all.
//   2. no record    the engine found nothing, and a rep who knows why may say
//                   so. This is the ONLY state that offers a control.
//   3. absolute     the subject objected, or the message cannot lawfully go.
//                   No control, because there is nothing anybody may press.
//
// TWO RULES HOLD THE DESIGN, and both are about what a rep learns from the
// screen rather than about the engine:
//
// NEVER RENDER A CONTROL THAT CANNOT BE USED. A disabled override on a refusal
// nobody may lift invites a support ticket and teaches reps that the product
// argues with them. State 3 offers nothing, deliberately.
//
// ALWAYS NAME WHO DECIDED. "Margince has no record" and "she asked us to stop"
// are different sentences and must never blur into "not allowed". A refusal
// that does not say whose it is reads as arbitrary, and reps route around
// arbitrary. This is why the wire carries `decided_by` at all.

type Preview = components["schemas"]["SendAuthorizationPreview"];
type Recipient = components["schemas"]["SendAuthorizationPreviewRecipient"];

/**
 * Which of the three states a preview puts the composer in.
 *
 * Exported so the states can be tested as a decision, separately from what they
 * render — and so a second surface adopting this component cannot reach a
 * fourth answer by branching on the preview itself.
 */
export type SendPermissionState =
  | "allowed"
  | "unproven"
  | "refused"
  | "checking";

/**
 * The recipient whose answer decides the message, and the state it puts the
 * composer in.
 *
 * The engine refuses a whole message for one refused recipient, so the composer
 * shows the STRONGEST objection rather than a list: a rep who fixes the first
 * of four problems and is then shown the second has been told the truth twice
 * and helped once. An absolute refusal outranks an unproven one because it is
 * the one the rep cannot act on.
 */
export function decidingRecipient(
  preview: Preview | undefined,
  asking = false,
): {
  state: SendPermissionState;
  recipient?: Recipient;
} {
  // AN ANSWER THAT HAS NOT ARRIVED IS NOT AN ANSWER. An absent preview used to
  // read as "allowed", so three situations reached one state: nobody asked, the
  // asking is in flight, and the answer came back clean. Only the last is
  // permission, and a rep who pressed Send in the second met the refusal at the
  // button — the failure this whole surface exists to end.
  //
  // An UNASKED question stays quiet rather than warning. A surface with no
  // recipient yet has nothing to ask about, and a warning there would tell a
  // rep something is wrong with a message they have not written.
  if (asking) return { state: "checking" };
  if (!preview) return { state: "allowed" };

  let overrulable: Recipient | undefined;
  for (const recipient of preview.recipients) {
    // `would_refuse` and not the verdict. Under a rollout mode short of enforce
    // a deny is recorded and the send still goes, so drawing the verdict would
    // show a rollout position as a rule and talk a rep out of lawful mail.
    if (!recipient.would_refuse) continue;
    // `can_be_overruled` and not `decided_by`, because the two disagree: a dead
    // mailbox and a rolling frequency cap are the engine's own reading AND
    // absolute. Offering the override there renders a button that cannot do
    // what it promises — the rep types a justification and staging refuses it
    // anyway. Absent means the server did not say, which is not permission to
    // guess yes.
    if (!recipient.can_be_overruled) {
      return { state: "refused", recipient };
    }
    overrulable ??= recipient;
  }
  if (overrulable) return { state: "unproven", recipient: overrulable };
  return { state: "allowed" };
}

/**
 * Says what the engine decided, and offers the override only where one exists.
 *
 * One component for every surface that stages a send — the composer, the
 * scheduled send and the held-draft approval card. A variant belongs here as a
 * prop; a second file would be the second spelling this design system has twice
 * grown by accident, and two spellings of a consent surface means one screen
 * tells a rep they may send while another says they may not, about one message.
 */
export function SendPermission({
  preview,
  unanswered = false,
  asking = false,
  onOverride,
}: Readonly<{
  preview: Preview | undefined;
  /**
   * The question is in flight. Drawn rather than left silent, because silence
   * here is what a permitted send looks like: the composer said nothing while
   * it waited and nothing when it was told yes, so a rep read the first as the
   * second.
   */
  asking?: boolean;
  /**
   * The question did not arrive: the preview failed rather than answered. Said
   * out loud, because a surface that fell silent here would look exactly like
   * one that was told yes — and a rep would learn of the refusal by pressing
   * Send, which is the failure this component exists to end.
   */
  unanswered?: boolean;
  /**
   * Offered only in the `unproven` state. Absent means the surface cannot take
   * an override — a read-only approval card, say — and the state renders as a
   * plain explanation rather than a dead control.
   */
  onOverride?: () => void;
}>) {
  const t = useT();
  const { state, recipient } = decidingRecipient(preview, asking);

  if (unanswered) {
    return (
      <p className="t-caption" role="status">
        {t("sendPermission.unanswered")}
      </p>
    );
  }

  if (state === "checking") {
    return (
      <p className="t-caption" role="status">
        {t("sendPermission.checking")}
      </p>
    );
  }

  if (state === "allowed") return null;

  if (state === "refused") {
    return (
      // `status` rather than the silence a standing notice derives: the answer
      // arrives as the rep picks a recipient, so one who does not hear it meets
      // the refusal at the Send button instead.
      <Callout
        tone="danger"
        kind="standing"
        live="status"
        icon={ShieldAlert}
        title={t("sendPermission.refused")}
      >
        {t(reasonKey(recipient))}
      </Callout>
    );
  }

  return (
    // `status` for the same reason the refusal above keeps it.
    <Callout
      tone="warn"
      kind="standing"
      live="status"
      icon={ShieldQuestion}
      title={t("sendPermission.unproven")}
      actions={
        onOverride ? (
          <Button onClick={onOverride}>{t("sendPermission.sayWhy")}</Button>
        ) : undefined
      }
    >
      {/* The hint says what the rep can DO, so it changes with what is on
          offer: with the control, how to answer; without it, what happens if
          the message goes as it stands. Telling a reader to "say so" beside no
          control would be the dead button in prose. */}
      {t(
        onOverride
          ? "sendPermission.unprovenHint"
          : "sendPermission.unprovenRefuses",
      )}
    </Callout>
  );
}

/**
 * The sentence for one refusal, chosen by the engine's reason code.
 *
 * A code this build does not recognise falls back to a sentence that is true of
 * every refusal and promises nothing specific. The alternative — rendering the
 * raw code — shows a rep `no_compatible_evidence` and asks them to interpret an
 * internal vocabulary.
 */
function reasonKey(recipient: Recipient | undefined) {
  switch (recipient?.reason_code) {
    case "marketing_objection":
      return "sendPermission.reason.objected" as const;
    case "consent_withdrawn":
      return "sendPermission.reason.withdrawn" as const;
    case "processing_restricted":
      return "sendPermission.reason.restricted" as const;
    case "subject_request":
      return "sendPermission.reason.askedUsToStop" as const;
    case "hard_bounce":
      return "sendPermission.reason.bounced" as const;
    case "frequency_cap_reached":
      return "sendPermission.reason.tooMany" as const;
    case "recipient_resolves_to_no_single_subject":
      return "sendPermission.reason.ambiguous" as const;
    case "unconfirmed_double_opt_in":
      return "sendPermission.reason.unconfirmed" as const;
    default:
      return "sendPermission.reason.other" as const;
  }
}

/**
 * SendMark is the quiet half of the answer: a mark that says the engine has
 * looked and found nothing wrong.
 *
 * SendPermission draws a Callout for the states a rep has to act on and
 * NOTHING for the state they do not. That silence was deliberate — the
 * overwhelming majority of sends are allowed and none should cost attention —
 * but it is also what the composer showed while it was still asking, and before
 * anybody had asked at all. Three situations, one blank space, and no way to
 * tell "checked and fine" from "not asked yet".
 *
 * So the allowed state gets a mark and the other two keep their Callout: this
 * renders beside the Send button, where the decision is made, and the Callout
 * stays in the body where an explanation belongs. A rep who never looks at the
 * mark loses nothing, which is the test a quiet signal has to pass.
 *
 * It draws NOTHING when there is no question yet. A mark over an empty form
 * would claim an answer about a message nobody has addressed.
 */
export function SendMark({
  preview,
  asking = false,
  unanswered = false,
}: Readonly<{
  preview: Preview | undefined;
  asking?: boolean;
  unanswered?: boolean;
}>) {
  const t = useT();
  const { state } = decidingRecipient(preview, asking);
  // The refused and unproven states are the Callout's, not the mark's: a
  // sentence a rep has to read does not belong in a glyph beside a button.
  if (unanswered || (state === "allowed" && !preview)) return null;
  if (state !== "allowed" && state !== "checking") return null;
  const label =
    state === "checking"
      ? t("sendPermission.checking")
      : t("sendPermission.ready");
  return (
    <CommunicationStatus
      state={state === "checking" ? "checking" : "ready"}
      scope="current_message"
      label={label}
      name={`${t("sendPermission.markName")}: ${label}`}
    />
  );
}
