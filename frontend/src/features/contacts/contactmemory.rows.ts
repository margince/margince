import type { useInteractionLabel } from "../../components/interactionchrome";
import { activityTimeline } from "../../design-system/activitytimeline";
import type { ReadingTone } from "../../design-system/readingtone";
import { groupChronology } from "../../design-system/timelinegroups";
import { primaryPhone } from "../../format/contacturi";
import { formatDayMonth } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  authorName,
  contactRole,
  owesOrIsOwed,
  writtenByPageContact,
  writtenByReader,
} from "../../lib/messagevoice";
import type { Activity, Contact360 } from "./contactmemory";
import type { OverviewRow } from "./contactoverview.rows";

type InteractionLabel = ReturnType<typeof useInteractionLabel>;
type Entry = NonNullable<Contact360["conversation_memory"]>[number];
type Status = Entry["status"];

/** One conversation as a card row, with the email it opens, if any. */
export type MemoryRow = OverviewRow &
  Readonly<{ kind: Activity["kind"]; emailId: string | null }>;

type Said = Readonly<{
  key: string;
  kind: Activity["kind"];
  provider: string | null;
  // The timeline row the conversation came from; a thread entry has none.
  row: Activity | null;
  occurredAt: string;
  title: string;
  status: Status;
  emailId: string | null;
}>;

/**
 * What was said lately, newest first. It comes from the thread projection when
 * the server has filled it, else from the timeline the page already read. The
 * timeline is folded into conversations the way the History tab folds it.
 */
export function memoryRows(
  view: Contact360,
  t: Translator,
  interactionLabel: InteractionLabel,
  locale: Locale,
  recordZone: string,
): MemoryRow[] {
  const entries = view.conversation_memory ?? [];
  const said = entries.length > 0 ? entries.map(fromEntry) : folded(view, t);
  return said.map((row) => ({
    key: row.key,
    title: row.title || interactionLabel(row.kind, row.provider),
    sub: [
      interactionLabel(row.kind, row.provider),
      directionWord(row, t),
      formatDayMonth(row.occurredAt, locale, recordZone),
    ]
      .filter(Boolean)
      .join(" · "),
    pill: statusPill(row.status, t),
    kind: row.kind,
    emailId: row.emailId,
  }));
}

export function callBackNumber(
  kind: Activity["kind"],
  contact: Contact360["contact"] | undefined,
): string | undefined {
  if (kind !== "call" || !contact || contact.archived_at) {
    return undefined;
  }
  return primaryPhone(contact.phones)?.phone;
}

function fromEntry(entry: Entry): Said {
  return {
    key: entry.key,
    kind: entry.channel,
    provider: entry.channel_provider ?? null,
    row: null,
    occurredAt: entry.occurred_at,
    title: entry.title,
    // An entry carries no author or page role, so it states no duty either.
    status: null,
    emailId:
      entry.channel === "email" ? (entry.first_activity_id ?? null) : null,
  };
}

function folded(view: Contact360, t: Translator): Said[] {
  const rows = (view.activities?.data ?? []).filter(
    (row) => row.kind !== "task" && !isFuture(row),
  );
  const byId = new Map(rows.map((row) => [row.id, row]));
  return groupChronology(activityTimeline(rows)).flatMap((group) => {
    const [newest] = group.entries;
    const row = newest && byId.get(newest.id);
    if (!row) return [];
    return {
      key: row.id,
      kind: row.kind,
      provider: row.channel_provider ?? null,
      row,
      occurredAt: row.occurred_at,
      // Withheld is read from the row's own state rather than trusted to have
      // been stripped, so a subject the server forgot to null never prints.
      title:
        row.content_state === "withheld"
          ? t("email.withheldSubject")
          : (row.email_summary?.subject ?? row.subject ?? ""),
      status: statusOf(row, rows),
      emailId: row.kind === "email" ? row.id : null,
    };
  });
}

// A meeting still to come belongs on the Next bar, not among what was said.
function isFuture(row: Activity): boolean {
  return new Date(row.occurred_at).getTime() > Date.now();
}

// Only an answer on this conversation settles it; a contact can owe several.
// A contact who was only copied owes and is owed nothing, as the server's
// email move already says.
function statusOf(row: Activity, rows: readonly Activity[]): Status {
  if (row.email_summary)
    return row.email_summary.move === "needs_reply" ? "unanswered" : null;
  if (!owesOrIsOwed(row)) {
    return null;
  }
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

// Who spoke. A meeting is both sides by its nature; a note has no side. A
// message names its recorded author. A call records who logged it, not who
// dialled, and a thread entry carries no author, so neither names anybody.
function directionWord(said: Said, t: Translator): string {
  if (!said.row) {
    return "";
  }
  if (said.kind === "meeting") {
    return t("contact.card.bothSides");
  }
  if (said.kind !== "email" && said.kind !== "message") {
    return "";
  }
  return authorWord(said.row, t);
}

function authorWord(row: Activity, t: Translator): string {
  const copied =
    contactRole(row) === "cc" ? ` · ${t("contact.card.inCc")}` : "";
  if (writtenByPageContact(row)) {
    return t("contact.card.theyWrote");
  }
  if (writtenByReader(row)) {
    return t("contact.card.youWrote") + copied;
  }
  const who = authorName(row);
  return who ? t("contact.card.wroteBy", { who }) + copied : "";
}

const STATUS: Readonly<
  Record<NonNullable<Status>, { key: MessageKey; tone: ReadingTone }>
> = {
  unanswered: { key: "contact.memory.unanswered", tone: "warning" },
  replied: { key: "contact.memory.replied", tone: "success" },
  awaiting_them: { key: "contact.card.awaitingThem", tone: "neutral" },
};

function statusPill(status: Status, t: Translator): OverviewRow["pill"] {
  if (!status) {
    return undefined;
  }
  const word = STATUS[status];
  return { text: t(word.key), tone: word.tone };
}
