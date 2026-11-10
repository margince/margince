import { ShieldAlert, ShieldQuestion } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CommunicationStatus } from "../design-system/communicationstatus";
import { formatNumber } from "../format/format";
import {
  type RefusalKind,
  sendPermissionReasonKey,
} from "../format/sendpermissionreason";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  decidingRefusal,
  previewStatus,
  type RefusedRecipient,
  refusalOf,
  refusedStatus,
  remedyCategoryOf,
  type SendStatus,
} from "../lib/sendstatus";

// What the engine decided about this message, said where the rep is writing it.
//
// A rep sees one status at a time: checking, ready, a missing detail, a
// missing fact, a missing permission, a stop, or an unfinished check. The engine's answer picks it, never the rep. A refused Send outranks the preview before it
// until the message changes or a newer check answers.
//
// Two rules hold the design, and both are about what a rep learns from the
// screen rather than about the engine:
//
// Never render a control that cannot be used. A disabled override on a refusal
// nobody may lift invites a support ticket and teaches reps that the product
// argues with them.
//
// Name what is missing. "Choose the deal for this follow-up" and "she
// asked us to stop" are different sentences and must never blur into "no
// consent". A refusal that does not say whose it is reads as arbitrary.

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

  // `would_refuse` and not the verdict. Under a rollout mode short of enforce
  // a deny is recorded and the send still goes. Drawing the verdict would talk
  // a rep out of lawful mail.
  const refusing = preview.recipients.filter((r) => r.would_refuse);
  // The same rule picks the deciding refusal after a refused Send.
  const refusals = refusing.map(refusalOf);
  const deciding = decidingRefusal(refusals);
  const recipient = deciding && refusing[refusals.indexOf(deciding)];
  if (!recipient) return { state: "allowed" };
  // `can_be_overruled` and not `decided_by`, because the two disagree: a dead
  // mailbox and a rolling frequency cap are the engine's own reading, and
  // absolute. Absent means the server did not say, which is not a yes.
  return {
    state: recipient.can_be_overruled ? "unproven" : "refused",
    recipient,
  };
}

/** A refusal a pressed Send came back with, about the message as it stands. */
export type SendRefusalAnswer = Readonly<{
  refusals: readonly RefusedRecipient[];
  /** What the server says this caller may do about it, already drawn. */
  remedies?: ReactNode;
}>;

type Answer = Readonly<{
  preview: Preview | undefined;
  asking?: boolean;
  unanswered?: boolean;
  /** A refused Send about this message. It outranks the preview it followed. */
  refusal?: SendRefusalAnswer | null;
}>;

/**
 * The one status a surface draws for the message: the refused Send when there
 * is one, else the preview. SendPermission and SendMark both read this, so the
 * mark beside Send never says ready while the body explains a refusal.
 */
export function sendStatus({
  preview,
  asking = false,
  unanswered = false,
  refusal,
}: Answer): SendStatus {
  if (refusal) return refusedStatus(refusal.refusals);
  return previewStatus({ preview, asking, unanswered });
}

const TITLE: Record<RefusalKind, MessageKey> = {
  missingContext: "sendPermission.missingContext",
  missingEvidence: "sendPermission.unproven",
  missingPermission: "sendPermission.missingPermission",
  restricted: "sendPermission.refused",
};

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
  refusal,
  onOverride,
  override,
  picksReason = false,
  namesRecipients = false,
}: Readonly<{
  preview: Preview | undefined;
  /**
   * The question is in flight. Drawn rather than left silent, because silence
   * here is what a permitted send looks like.
   */
  asking?: boolean;
  /**
   * The question did not arrive: the preview failed rather than answered. Said
   * out loud, because a surface that fell silent here would look exactly like
   * one that was told yes.
   */
  unanswered?: boolean;
  /** A refused Send about this message, with the remedies the server offers. */
  refusal?: SendRefusalAnswer | null;
  /**
   * Offered only where the preview's deciding recipient may be overruled.
   * Absent means the surface cannot take an override, as on a read-only
   * approval card. The state then renders as a plain explanation.
   */
  onOverride?: () => void;
  /**
   * A control that records the override where it is read, drawn in place of
   * the button `onOverride` asks for. Same state, same rule: absent, no control.
   */
  override?: ReactNode;
  /** The surface also draws the message's own reason-for-contact pick. */
  picksReason?: boolean;
  /** The viewer wrote every address, so the refused one may be named. */
  namesRecipients?: boolean;
}>) {
  const t = useT();
  const status = sendStatus({ preview, asking, unanswered, refusal });
  if (status.kind === "unavailable") {
    return <p role="status">{t("sendPermission.unanswered")}</p>;
  }
  if (status.kind === "checking") {
    return <p role="status">{t("sendPermission.checking")}</p>;
  }
  if (status.kind === "idle" || status.kind === "ready") return null;

  // The override answers the preview's deciding recipient. A refused Send
  // carries the server's own remedies instead.
  const overrulable =
    !refusal && decidingRecipient(preview, asking).state === "unproven";
  const control = overrulable
    ? (override ??
      (onOverride ? (
        <Button onClick={onOverride}>{t("sendPermission.sayWhy")}</Button>
      ) : undefined))
    : undefined;
  const surface = { recordable: Boolean(control), picksReason };
  const deciding = status.deciding;
  return (
    <Callout
      tone={status.kind === "restricted" ? "danger" : "warning"}
      kind={refusal ? "outcome" : "standing"}
      // `status` while the rep writes, so one who does not hear the answer
      // does not meet it at the Send button. A refused Send is an alert.
      live={refusal ? "alert" : "status"}
      icon={status.kind === "restricted" ? ShieldAlert : ShieldQuestion}
      title={t(TITLE[status.kind])}
      actions={control ?? refusal?.remedies}
    >
      {/* The hint says what the rep can DO, so it changes with what is on
          offer: with the control, how to answer; without it, what happens if
          the message goes as it stands. */}
      <p>
        {t(
          sendPermissionReasonKey(
            deciding?.reasonCode,
            surface,
            deciding && remedyCategoryOf(deciding),
          ),
        )}
      </p>
      <AffectedRecipients status={status} named={namesRecipients} />
    </Callout>
  );
}

// Which recipients the explanation is about. A message to several recipients
// must not read as if one recipient's refusal were everybody's.
function AffectedRecipients({
  status,
  named,
}: Readonly<{ status: SendStatus; named: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const others = status.refusing - 1;
  if (named && status.deciding) {
    const address = status.deciding.address;
    return (
      <p className="t-caption">
        {others > 0
          ? plural("sendPermission.appliesToAndMore", others, {
              address,
              count: formatNumber(others, locale),
            })
          : t("sendPermission.appliesTo", { address })}
      </p>
    );
  }
  if (status.recipients > 1 && status.refusing > 0) {
    return (
      <p className="t-caption">
        {plural("sendPermission.appliesToSome", status.refusing, {
          count: formatNumber(status.refusing, locale),
          total: formatNumber(status.recipients, locale),
        })}
      </p>
    );
  }
  return null;
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
  refusal,
  blocked = false,
}: Answer &
  Readonly<{
    /** Something the surface knows will stop the send, such as an empty field. */
    blocked?: boolean;
  }>) {
  const t = useT();
  const { kind } = sendStatus({ preview, asking, unanswered, refusal });
  // Every other state is the Callout's: a sentence a rep has to read does not
  // belong in a glyph beside a button. A blocked form makes no passed claim.
  if (kind !== "checking" && (kind !== "ready" || blocked)) return null;
  const label = t(
    kind === "checking" ? "sendPermission.checking" : "sendPermission.ready",
  );
  return (
    <CommunicationStatus
      state={kind}
      scope="current_message"
      label={label}
      name={`${t("sendPermission.markName")}: ${label}`}
    />
  );
}
