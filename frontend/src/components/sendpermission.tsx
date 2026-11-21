import { ShieldAlert, ShieldQuestion } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Row } from "../design-system/stack";
import { footerFindingsText } from "../format/footerfindings";
import { formatNumber } from "../format/format";
import {
  type RefusalKind,
  type RefusalSurface,
  refusalSentenceKey,
} from "../format/sendpermissionreason";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  previewStatus,
  type RefusedRecipient,
  refusedStatus,
  remedyCategoryOf,
  type SendStatus,
} from "../lib/sendstatus";
import { CitedLaws } from "./citedlaw";
import { lawsOf } from "./citedprovisions";
import { LawfulBasisCallout } from "./lawfulbasiswarning";

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
// Name what is missing. "This follow-up has no open deal" and "she
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
  // The server ranks refusals, so the first is the deciding one, as it is
  // after a refused Send.
  const recipient = refusing[0];
  if (!recipient) return { state: "allowed" };
  // `can_be_overruled` and not `decided_by`: a frequency cap is the engine's
  // own reading, yet one message may pass it with a reason. Absent is not a yes.
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
 * is one, else the preview.
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

/**
 * A refusal a recorded reason can pass reads as a warning. One nothing lifts,
 * such as the recipient's own stop, reads as a danger, under "Message cannot be sent".
 */
function calloutLook(kind: RefusalKind, overrulable: boolean) {
  const hardStop = kind === "restricted" && !overrulable;
  const title: MessageKey =
    kind === "restricted" && !hardStop
      ? "sendPermission.stopWithReason"
      : TITLE[kind];
  return hardStop
    ? { tone: "danger" as const, icon: ShieldAlert, title }
    : { tone: "warning" as const, icon: ShieldQuestion, title };
}

const TITLE: Record<RefusalKind, MessageKey> = {
  missingContext: "sendPermission.missingContext",
  missingEvidence: "sendPermission.unproven",
  missingPermission: "sendPermission.missingPermission",
  missingFooter: "sendPermission.footer.title",
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
type SendPermissionProps = Readonly<{
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
  /** The message was checked with no reason for contact of its own. */
  unclaimed?: boolean;
  /** The viewer wrote every address, so the refused one may be named. */
  namesRecipients?: boolean;
  /**
   * Given where Send waits for the answer, so a failed check is asked again
   * here rather than by a press that cannot be made.
   */
  onRecheck?: () => void;
}>;

/**
 * The engine's answer for this message, and the warning a recipient with no
 * lawful basis on record owes. The warning stands even when the message may go.
 */
export function SendPermission(props: SendPermissionProps) {
  return (
    <>
      <LawfulBasisCallout preview={props.preview} />
      <PermissionNotice {...props} />
    </>
  );
}

function PermissionNotice({
  preview,
  unanswered = false,
  asking = false,
  refusal,
  onOverride,
  override,
  picksReason = false,
  unclaimed = false,
  namesRecipients = false,
  onRecheck,
}: SendPermissionProps) {
  const t = useT();
  const { locale } = useLocale();
  const status = sendStatus({ preview, asking, unanswered, refusal });
  if (status.kind === "unavailable") {
    return <UncheckedNotice onRecheck={onRecheck} />;
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
  const surface = { recordable: Boolean(control), picksReason, unclaimed };
  const look = calloutLook(
    status.kind,
    status.deciding?.canBeOverruled === true,
  );
  return (
    <Callout
      tone={look.tone}
      kind={refusal ? "outcome" : "standing"}
      // `status` while the rep writes, so one who does not hear the answer
      // does not meet it at the Send button. A refused Send is an alert.
      live={refusal ? "alert" : "status"}
      icon={look.icon}
      title={t(look.title)}
      actions={control ?? refusal?.remedies}
    >
      {status.refused.some((item) => item.footerFindings?.length) ? (
        <p>
          {footerFindingsText(
            [
              ...new Set(
                status.refused.flatMap((item) => item.footerFindings ?? []),
              ),
            ],
            t,
            locale,
          )}
        </p>
      ) : null}
      <CitedLawsOf refused={status.refused} />
      <RefusalDetail
        status={status}
        named={namesRecipients}
        surface={surface}
      />
    </Callout>
  );
}

// The law each refusal and its footer findings cite, after the facts.
function CitedLawsOf({
  refused,
}: Readonly<{ refused: readonly RefusedRecipient[] }>) {
  const provisions = refused.flatMap((item) => [
    ...lawsOf(item),
    ...(item.footerFindings ?? []).flatMap(lawsOf),
  ]);
  if (!provisions.some(Boolean)) return null;
  return (
    <p>
      <CitedLaws provisions={provisions} />
    </p>
  );
}

// A check that did not complete. Where Send waits on it, it is asked again here.
function UncheckedNotice({ onRecheck }: Readonly<{ onRecheck?: () => void }>) {
  const t = useT();
  if (!onRecheck) {
    return <p role="status">{t("sendPermission.unanswered")}</p>;
  }
  return (
    <Row>
      <p role="status">{t("sendPermission.unansweredHeld")}</p>
      <Button onClick={onRecheck}>{t("sendPermission.recheck")}</Button>
    </Row>
  );
}

// What the refusal means and whom it is about. The hint names the rep's next
// step, so it follows what is on offer. With the control it says how to answer.
// Without it, it says what happens if the message goes as it stands.
// When several recipients the viewer wrote are refused, each gets its own line,
// so one refusal never reads as everybody's.
function RefusalDetail({
  status,
  named,
  surface,
}: Readonly<{
  status: SendStatus;
  named: boolean;
  surface: RefusalSurface;
}>) {
  const t = useT();
  const hintFor = (refused: RefusedRecipient | undefined) =>
    t(
      refusalSentenceKey(
        refused?.reasonCode,
        surface,
        refused && remedyCategoryOf(refused),
      ),
    );
  if (named && status.refused.length > 1) {
    return (
      <ul className="t-caption">
        {status.refused.map((refused) => (
          <li key={refused.address}>
            {t("sendPermission.recipientLine", {
              address: refused.address,
              reason: hintFor(refused),
            })}
          </li>
        ))}
      </ul>
    );
  }
  return (
    <>
      <p>{hintFor(status.deciding)}</p>
      <AffectedRecipients status={status} named={named} />
    </>
  );
}

function AffectedRecipients({
  status,
  named,
}: Readonly<{ status: SendStatus; named: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (named && status.deciding) {
    return (
      <p className="t-caption">
        {t("sendPermission.appliesTo", { address: status.deciding.address })}
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
