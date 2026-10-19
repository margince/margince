import { activityTimeline } from "../../design-system/activitytimeline";
import type { RecordContact } from "../../design-system/participants";
import { groupChronology } from "../../design-system/timelinegroups";
import { formatDayMonth, formatTimeOfDay } from "../../format/format";
import type { Locale, useT } from "../../i18n";
import type { useInteractionLabel } from "../records/interactionchrome";
import type {
  Activity,
  Contact360,
  EmailSummary,
  Filter,
} from "./contactmemory";

type Row = {
  messageCount?: number;
  key: string;
  // Who the row is drawn beside: the other party on a retained email, else the
  // contact. A note or a call has no second name of its own to show. `key` is a
  // contact's id, or the counterparty's name when it names no known contact.
  who: Readonly<{ name: string; key: string }>;
  date: string;
  time: string;
  // What a reply anchors on, and the transport it would leave by. Null when the
  // row is not a channel message, or the entry names no activity. A thread
  // projection need not carry one, and a reply anchored on nothing can only fail.
  activityId: string | null;
  kind: Activity["kind"] | null;
  channelProvider: string | null;
  channel: string;
  channelLabel: string;
  title: string;
  summary: string;
  // The server's own row model for a retained email, when this row is one.
  // When present, the card draws EmailEntry instead of its own two lines. A
  // message then reads here as it does on the timeline beside it.
  //
  // Null on every other kind, and for a thread-projection entry too. A
  // conversation summary folds several messages, and has no single message's
  // access state to show.
  emailSummary: EmailSummary | null;
  // `status` stays the stored key and `statusLabel` is what the reader sees.
  // Folding them into one field would make the badge's tone depend on the
  // active locale, since tone is chosen by the same word.
  status: string | null;
  statusLabel: string;
  tone: "success" | "warning" | "accent" | undefined;
};

// The row's opener, or nothing.
//
// Both halves are required and neither is assumed. A page that mounts no
// drawer passes no opener; a row with no activity id has nothing to open.
// EmailEntry draws an un-openable row as plain text. A row that looks openable
// and is not teaches a reader the product does not work.
export function openerFor(
  row: Row,
  onOpenEmail: ((activityId: string) => void) | undefined,
): (() => void) | undefined {
  const { activityId } = row;
  if (!onOpenEmail || !activityId) {
    return undefined;
  }
  return () => onOpenEmail(activityId);
}

// The filter key for the channel column. A message names its transport in its
// own field (channel_provider), apart from its kind. The kind alone renders every channel row as the bare
// word "message", so a Telegram and a Dispact thread look the same. Folding the
// provider in keeps them apart; a row with no transport keys on its kind.
//
// It keys the segmented filter and nothing a reader sees. The name comes from
// the transport directory and the icon from the kind. So an unknown provider
// still reads, and no row is drawn as something it is not.
function channelKeyOf(
  kind: string,
  provider: string | null | undefined,
): string {
  return provider ?? kind;
}

// interactionLabel is useInteractionLabel's resolver, threaded in rather than
// called here: a Row is built outside the component, and a hook may not be.
// recordZone rides in the same way, for the same reason: useRecordZone is a
// hook, so the zone is read once in ContactMemory and passed down.
type InteractionLabel = ReturnType<typeof useInteractionLabel>;

// The row's counterparty, or this card's contact when the message names none.
// A mail naming this contact keys on its record, as a thread's face does; any
// other name, a colleague's or a stranger's, keys on itself.
function whoFor(
  activity: Pick<Activity, "email_summary" | "links"> | null,
  contact: RecordContact,
): Row["who"] {
  const counterparty = activity?.email_summary?.counterparty;
  if (!counterparty) return { name: contact.full_name, key: contact.id };
  // The server's own answer for which contact the phrase names. Matching the
  // phrase by name would miss a contact renamed since capture. It also could
  // not tell two contacts sharing a name apart.
  const named = activity?.email_summary?.counterparty_contact_id;
  return { name: counterparty, key: named ?? counterparty };
}

export function fromEntry(
  entry: NonNullable<Contact360["conversation_memory"]>[number],
  t: ReturnType<typeof useT>,
  interactionLabel: InteractionLabel,
  locale: Locale,
  recordZone: string,
  contact: RecordContact,
): Row {
  const status = entry.status ?? null;
  return {
    key: entry.key,
    // A thread-projection entry carries no email summary of its own, so the
    // counterparty it reads is always the contact this card is on.
    who: whoFor(null, contact),
    date: formatDayMonth(entry.occurred_at, locale, recordZone),
    time: formatTimeOfDay(entry.occurred_at, locale, recordZone),
    // first_activity_id is what "expand to original" opens, and it anchors a
    // reply too. The send resolves the conversation from the anchor's own links
    // and thread key. So a thread's first message names the same conversation
    // as its last.
    activityId: entry.first_activity_id ?? null,
    // The row's own kind, whatever it is. A reply is offered on a mail row as on
    // a channel one, the rule the 360 timelines apply. A composer gated to
    // channel rows would leave a mail-only workspace unable to answer from here.
    kind: entry.channel,
    channelProvider: entry.channel_provider ?? null,
    channel: channelKeyOf(entry.channel, entry.channel_provider),
    channelLabel: interactionLabel(entry.channel, entry.channel_provider),
    title: entry.title,
    summary: entry.summary,
    // A conversation entry folds a whole thread, so no single message's access
    // state describes it. It keeps the card's own two lines.
    emailSummary: null,
    status,
    statusLabel: statusLabel(status, t),
    tone: toneFor(status),
  };
}

// Without a generated summary, reuse the chronology’s conversation grouping.
export function foldActivities(
  view: Contact360,
  t: ReturnType<typeof useT>,
  interactionLabel: InteractionLabel,
  locale: Locale,
  recordZone: string,
): Row[] {
  const rows = (view.activities?.data ?? []).filter(
    (row) => row.kind !== "task" && !isFuture(row),
  );
  const byId = new Map(rows.map((row) => [row.id, row]));
  return groupChronology(activityTimeline(rows)).flatMap((group) => {
    const [newest] = group.entries;
    const row = newest && byId.get(newest.id);
    if (!row) return [];
    const status = statusOf(row, rows);
    const withheldRow = row.content_state === "withheld";
    return {
      key: row.id,
      messageCount: group.entries.length,
      who: whoFor(row, view.contact),
      date: formatDayMonth(row.occurred_at, locale, recordZone),
      time: formatTimeOfDay(row.occurred_at, locale, recordZone),
      activityId: row.id,
      kind: row.kind,
      channelProvider: row.channel_provider ?? null,
      channel: channelKeyOf(row.kind, row.channel_provider),
      channelLabel: interactionLabel(row.kind, row.channel_provider),
      // Withheld is read from the row's own state rather than trusted to
      // have been stripped. The server does strip it: both readers of the
      // activity table null the subject and the body together. Yet this card
      // would otherwise fall through to row.subject. A fallback chain that
      // holds only because of what the server sends is one response away from
      // printing a subject it should not.
      title: withheldRow
        ? t("email.withheldSubject")
        : (row.email_summary?.subject ??
          row.subject ??
          interactionLabel(row.kind, row.channel_provider)),
      // The server's own preview for an email, composed with the same
      // splitter the drawer folds with. So this card and the message it
      // opens cannot disagree about where the sender's words end.
      summary: withheldRow
        ? ""
        : row.email_summary
          ? (row.email_summary.preview ?? "")
          : (row.body ?? ""),
      // A retained email draws the canonical row. The title and summary
      // above stay filled for it. The segmented filter reads them, and the
      // row falls back to them if a server has not caught up.
      emailSummary: row.email_summary ?? null,
      status,
      statusLabel: statusLabel(status, t),
      tone: toneFor(status),
    };
  });
}

// A meeting still to come belongs on the strip and in the Today card. Repeating
// it here as something remembered would be wrong.
function isFuture(row: Activity): boolean {
  return new Date(row.occurred_at).getTime() > Date.now();
}

// Only an answer on this conversation settles it; a contact can owe several replies.
function statusOf(row: Activity, rows: readonly Activity[]): string | null {
  if (row.email_summary)
    return row.email_summary.move === "needs_reply" ? "unanswered" : null;
  if (row.direction === "inbound") {
    const answered =
      row.thread_key != null &&
      rows.some(
        (other) =>
          other.thread_key === row.thread_key &&
          other.kind === row.kind &&
          other.channel_provider === row.channel_provider &&
          other.direction === "outbound" &&
          new Date(other.occurred_at) > new Date(row.occurred_at),
      );
    return answered ? "replied" : "unanswered";
  }
  return null;
}

// A status this client has no word for is shown as it was stored rather than
// dropped. An unlabelled badge is still evidence. An absent one claims that the
// exchange has no status.
function statusLabel(
  status: string | null,
  t: ReturnType<typeof useT>,
): string {
  switch (status) {
    case "replied":
      return t("contact.memory.replied");
    case "unanswered":
      return t("contact.memory.unanswered");
    default:
      return status ?? "";
  }
}

function toneFor(
  status: string | null,
): "success" | "warning" | "accent" | undefined {
  switch (status) {
    case "replied":
      return "success";
    case "unanswered":
      return "warning";
    case "awaiting_them":
      return "accent";
    default:
      return undefined;
  }
}

export function matches(row: Row, filter: Filter): boolean {
  switch (filter) {
    case "all":
      return true;
    case "email":
      return row.channel === "email";
    case "meetings":
      return row.channel === "meeting";
    case "calls":
      return row.channel === "call";
    case "notes":
      return row.channel === "note";
    default:
      return true;
  }
}
