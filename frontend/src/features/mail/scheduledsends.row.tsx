import { useState } from "react";
import { Badge, Button, Card, TextInput } from "../../design-system/atoms";
import { Row } from "../../design-system/stack";
import { localDateTimeValue } from "../../format/calendarday";
import {
  formatDateTime,
  formatNumber,
  isRenderableZone,
} from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { logUnexpectedError } from "../../lib/problem";
import { scheduleFields } from "./composesend";
import type { Move, ScheduledSend, Status } from "./scheduledsends";
import { SendPermission } from "./sendpermission";
import { useSendPermission } from "./usesendpermission";

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
 */
function Moment({
  send,
  readerZone,
}: Readonly<{ send: ScheduledSend; readerZone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = renderZone(send.scheduled_tz, readerZone);
  return (
    <span className="t-caption">
      {formatDateTime(send.scheduled_at, locale, zone)}
      {zone !== readerZone && ` · ${t("sched.inZone", { zone })}`}
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

/**
 * The reschedule control: a moment picker that unfolds in place.
 *
 * Time only, which is the contract's own rule. The approval bound to the
 * content, so changing it is cancel-and-recompose. The picker is a
 * `datetime-local`, seeded from the send's moment in the reader's zone. The
 * imported `scheduleFields` reads it back in that zone, as the composer's
 * send-later control does, so the two agree on what a picked moment means.
 */
function MoveControl({
  send,
  pending,
  onMove,
}: Readonly<{
  send: ScheduledSend;
  pending: boolean;
  onMove: (move: Move) => void;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");

  if (!open) {
    return (
      <Button
        onClick={() => {
          setDraft(localDateTimeValue(send.scheduled_at));
          setOpen(true);
        }}
      >
        {t("sched.move")}
      </Button>
    );
  }
  // Empty or unparseable yields neither field, and the wire requires both. The
  // button refuses rather than sending half a schedule the server would 422.
  const fields = scheduleFields(draft);
  return (
    <>
      <TextInput
        type="datetime-local"
        aria-label={t("sched.moveTo", { subject: send.subject })}
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        className="scheduledsends-move"
      />
      <Button
        variant="primary"
        disabled={fields.scheduled_at === undefined}
        pending={pending}
        onClick={() => {
          if (fields.scheduled_at === undefined) return;
          onMove({
            id: send.id,
            version: send.version,
            at: fields.scheduled_at,
          });
          setOpen(false);
        }}
      >
        {t("sched.moveSave")}
      </Button>
      <Button onClick={() => setOpen(false)}>{t("sched.moveCancel")}</Button>
    </>
  );
}

export function SendRow({
  send,
  readerZone,
  movePending,
  onMove,
  onWithdraw,
}: Readonly<{
  send: ScheduledSend;
  readerZone: string;
  movePending: boolean;
  onMove: (move: Move) => void;
  onWithdraw: (send: ScheduledSend) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const actionable = STILL_MOVABLE.has(send.status);
  const heldReasonKey = send.held_reason
    ? HELD_REASON_LABEL[send.held_reason]
    : undefined;
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
    <Card as="div" className="scheduledsends-send">
      <Row gap="2">
        <span className="scheduledsends-summary">
          <strong>{send.subject}</strong>
          <span className="t-caption"> · {recipientLine(send, t, locale)}</span>
          <br />
          <Moment send={send} readerZone={readerZone} />
        </span>
        <Badge tone={STATUS_TONE[send.status]}>
          {t(STATUS_LABEL[send.status])}
        </Badge>
        {actionable && (
          <MoveControl send={send} pending={movePending} onMove={onMove} />
        )}
        {actionable && (
          <Button variant="danger" onClick={() => onWithdraw(send)}>
            {t("sched.withdraw")}
          </Button>
        )}
      </Row>
      {/* A held message says why on the row rather than in a tooltip. It is the
          only thing that tells the rep whether to move it or to give up on it,
          and a reason a rep has to hover for is a reason they do not read. An
          unmapped token prints nothing rather than the token, since a reason
          nobody can act on is worse than the sentence above it standing alone. */}
      {heldReasonKey && (
        <p className="scheduledsends-held">{t(heldReasonKey)}</p>
      )}
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
    </Card>
  );
}
