import { useId } from "react";
import { SendPermission } from "../../components/sendpermission";
import { Badge, Button, Card } from "../../design-system/atoms";
import {
  formatDateTime,
  formatNumber,
  formatTimeOfDay,
  isRenderableZone,
} from "../../format/format";
import { INTL_LOCALE } from "../../format/intllocale";
import { zoneNameAndOffset } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { logUnexpectedError } from "../../lib/problem";
import { useSendPermission } from "../../lib/usesendpermission";
import { dropsOperatorReason, isEditable } from "./schedulededit";
import type { ScheduledSend, Status } from "./scheduledqueue";
import { type Move, RescheduleMenu } from "./scheduledsends.reschedule";

// What each status says, in the five-state vocabulary. `scheduled` is work
// still in flight, `sent` is the favourable outcome, `held` needs a human
// before it can go, and `cancelled` is a send that will never happen. Each has
// its own tone, so a send that landed and one a rep withdrew never look alike.
const STATUS_TONE: Record<Status, "info" | "success" | "warning" | undefined> =
  {
    scheduled: "info",
    released: "info",
    sent: "success",
    cancelled: undefined,
    held: "warning",
  };

const STATUS_LABEL: Record<Status, MessageKey> = {
  scheduled: "sched.status.scheduled",
  released: "sched.status.released",
  sent: "sched.status.sent",
  cancelled: "sched.status.cancelled",
  held: "sched.status.held",
};

// Why a human has to look at it, in the contract's own closed set. Rendered as
// words because `held_reason` is a wire token and a rep cannot act on
// `timer_exhausted`.
const HELD_REASON_LABEL: Record<string, MessageKey> = {
  consent_withdrawn: "sched.held.consentWithdrawn",
  sender_inactive: "sched.held.senderInactive",
  missed_window: "sched.held.missedWindow",
  timer_exhausted: "sched.held.timerExhausted",
  send_refused: "sched.held.sendRefused",
  passport_revoked: "sched.held.passportRevoked",
  followup_settled: "sched.held.followupSettled",
  meeting_canceled: "sched.held.meetingCanceled",
};

// Which statuses the server will still let a rep move or withdraw. It mirrors
// the store's own `status IN ('scheduled','held')` predicate, and it is spelled
// here so a row cannot offer a verb the write is about to refuse.
const STILL_MOVABLE: ReadonlySet<Status> = new Set<Status>([
  "scheduled",
  "held",
]);

/**
 * The zone a send's moment is rendered in.
 *
 * The wire carries the IANA zone the human picked the moment in, kept so it
 * re-renders as meant. `formatDateTime` throws on anything that is not an IANA
 * name, a fixed offset included. An offset freezes the DST rules of the day it
 * was picked. On this page that throw would take the whole list down over one row.
 *
 * So a zone this runtime cannot resolve falls back to the reader's own and is
 * logged. That is a claim about the server, not the reader: a conforming one
 * cannot produce it. Without the log, a fork's malformed zone would go
 * unnoticed while every moment on the page shifted.
 *
 * The question is asked with `isRenderableZone`, the formatter's own predicate,
 * and not by probing Intl here. Probing Intl asks whether the name resolves,
 * which every fixed offset does. So `Etc/GMT-1`, `GMT` and `+01:00` would pass
 * that probe and then throw inside `formatDateTime`, in the case the fallback
 * exists for.
 */
function renderZone(wireZone: string, readerZone: string): string {
  if (isRenderableZone(wireZone)) {
    return wireZone;
  }
  logUnexpectedError(
    new Error(`scheduled_tz is not a renderable IANA zone: "${wireZone}"`),
  );
  return readerZone;
}

/**
 * One row's moment, in the zone it was chosen in, naming that zone when it is
 * not the reader's own.
 *
 * A rep schedules 09:00 from Berlin and reads the page from Hanoi. Told 14:00
 * with no explanation, they read a bug. Told 09:00 with no zone, they read a
 * message that will land at the wrong end of the recipient's morning.
 *
 * `timeOnly` is for a row under a day heading, where the date is already said.
 */
function Moment({
  send,
  readerZone,
  timeOnly,
}: Readonly<{ send: ScheduledSend; readerZone: string; timeOnly: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = renderZone(send.scheduled_tz, readerZone);
  const when =
    timeOnly && zone === readerZone
      ? formatTimeOfDay(send.scheduled_at, locale, zone)
      : formatDateTime(send.scheduled_at, locale, zone);
  return (
    <span className="scheduledsends-moment">
      {when}
      {zone !== readerZone && (
        <span className="t-caption">
          {t("sched.inZone", {
            zone: zoneNameAndOffset(
              INTL_LOCALE[locale],
              new Date(send.scheduled_at),
              zone,
            ),
          })}
        </span>
      )}
    </span>
  );
}

/** The To line, with the rest counted rather than listed. */
function recipientLine(
  send: ScheduledSend,
  t: Translator,
  locale: Locale,
): string {
  const [first, ...rest] = send.to;
  if (!first) {
    // Required on the wire and non-empty in every path that writes it. Stated
    // rather than rendered as an empty cell, which reads as a message addressed
    // to nobody.
    return t("sched.recipientsUnknown");
  }
  return rest.length === 0
    ? first
    : t("sched.recipientsMore", {
        first,
        count: formatNumber(rest.length, locale),
      });
}

/** Why the row's Edit is refused, as one line the button points at. */
function EditRefusalLine({
  send,
  id,
}: Readonly<{ send: ScheduledSend; id: string }>) {
  const t = useT();
  if (!dropsOperatorReason(send)) {
    return null;
  }
  return (
    <span id={id} className="t-caption">
      {t("sched.editRefused.operatorReason")}
    </span>
  );
}

function EditVerb({
  send,
  refusalId,
  onEdit,
}: Readonly<{
  send: ScheduledSend;
  refusalId: string;
  onEdit: (send: ScheduledSend) => void;
}>) {
  const t = useT();
  const refused = dropsOperatorReason(send);
  if (!refused && !isEditable(send)) {
    return null;
  }
  return (
    <Button
      variant="ghost"
      reasonId={refused ? refusalId : undefined}
      onClick={() => onEdit(send)}
    >
      <span aria-hidden>{t("sched.edit")}</span>
      <span className="sr-only">
        {t("sched.editFor", { subject: send.subject })}
      </span>
    </Button>
  );
}

/** The first line of a message, for a row that says what the message is. */
function previewOf(body: string | undefined): string | null {
  const line = body?.trim().split(/\s*\n\s*/)[0];
  return line ? line : null;
}

export function SendRow({
  send,
  readerZone,
  now,
  timeOnly,
  isNext,
  movePending,
  onMove,
  onEdit,
  onWithdraw,
}: Readonly<{
  send: ScheduledSend;
  readerZone: string;
  now: Date;
  timeOnly: boolean;
  isNext: boolean;
  movePending: boolean;
  onMove: (move: Move) => void;
  onEdit: (send: ScheduledSend) => void;
  onWithdraw: (send: ScheduledSend) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const refusalId = useId();
  const actionable = STILL_MOVABLE.has(send.status);
  const heldReasonKey = send.held_reason
    ? HELD_REASON_LABEL[send.held_reason]
    : undefined;
  const preview = previewOf(send.body);
  // What the engine says about this message now, for a message that will still
  // go. It is asked the way the fire asks: every addressee, the frozen claim and
  // the frozen records. So the row cannot say "will send" about a message the
  // fire then holds. A closed row is not asked: nothing about it will change on
  // its own, and a verdict on a cancelled message is noise.
  const permission = useSendPermission({
    recipients: [...send.to, ...(send.cc ?? []), ...(send.bcc ?? [])],
    anchorActivityId: send.anchor_activity_id ?? undefined,
    links: send.links,
    context: send.communication_context,
    marketingPurpose: send.marketing_purpose,
    consentPurpose: send.consent_purpose,
    evidence: send.evidence,
    enabled: actionable,
  });
  return (
    <Card
      as="li"
      className={[
        "scheduledsends-send",
        isNext && "scheduledsends-send-next",
        send.status === "held" && "scheduledsends-send-held",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <Moment send={send} readerZone={readerZone} timeOnly={timeOnly} />
      <div className="scheduledsends-summary">
        <strong>{send.subject}</strong>
        <span className="t-caption">{recipientLine(send, t, locale)}</span>
        {preview && (
          <span className="scheduledsends-preview t-caption">{preview}</span>
        )}
        {/* A held message says why on the row rather than in a tooltip. It is the
            only thing that tells the rep whether to move it or to give up on it,
            and a reason a rep has to hover for is a reason they do not read. An
            unmapped token prints nothing rather than the token, since a reason
            nobody can act on is worse than the sentence above it standing alone. */}
        {heldReasonKey && (
          <p className="scheduledsends-held">{t(heldReasonKey)}</p>
        )}
        {actionable && <EditRefusalLine send={send} id={refusalId} />}
        {/* The held reason above says a gate stopped it; this says WHOSE decision
            that was and whether anybody may change it, in the same words the
            composer uses. Read-only: the queue moves or withdraws a message, and
            an override belongs where the message is written. */}
        {actionable && (
          <SendPermission
            preview={permission.preview}
            asking={permission.asking}
            unanswered={permission.unanswered}
          />
        )}
      </div>
      <div className="scheduledsends-side">
        {isNext ? (
          <Badge tone="accent">{t("sched.nextBadge")}</Badge>
        ) : (
          send.status !== "scheduled" && (
            <Badge tone={STATUS_TONE[send.status]}>
              {t(STATUS_LABEL[send.status])}
            </Badge>
          )
        )}
        {actionable && (
          <div className="scheduledsends-verbs">
            <RescheduleMenu
              send={send}
              now={now}
              pending={movePending}
              onMove={onMove}
            />
            <EditVerb send={send} refusalId={refusalId} onEdit={onEdit} />
            <Button variant="danger" onClick={() => onWithdraw(send)}>
              <span aria-hidden>{t("sched.withdraw")}</span>
              <span className="sr-only">
                {t("sched.withdrawFor", { subject: send.subject })}
              </span>
            </Button>
          </div>
        )}
      </div>
    </Card>
  );
}
