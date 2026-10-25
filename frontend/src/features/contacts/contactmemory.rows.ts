import type { useInteractionLabel } from "../../components/interactionchrome";
import { activityTimeline } from "../../design-system/activitytimeline";
import type { ReadingTone } from "../../design-system/readingtone";
import { groupChronology } from "../../design-system/timelinegroups";
import {
  activityDay,
  activityHasHappened,
  type OccurredPrecision,
} from "../../format/activitywhen";
import { primaryPhone } from "../../format/contacturi";
import { formatDayMonth } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
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
  direction: Entry["direction"] | null | undefined;
  occurredAt: string;
  occurredPrecision: OccurredPrecision;
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
  const said =
    entries.length > 0 ? entries.map(fromEntry) : folded(view, t, recordZone);
  return said.map((row) => ({
    key: row.key,
    title: row.title || interactionLabel(row.kind, row.provider),
    sub: [
      interactionLabel(row.kind, row.provider),
      directionWord(row.kind, row.direction, t),
      activityDay(
        row.occurredAt,
        row.occurredPrecision,
        locale,
        recordZone,
        formatDayMonth,
      ),
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
    direction: entry.direction,
    occurredAt: entry.occurred_at,
    occurredPrecision: entry.occurred_precision,
    title: entry.title,
    status: entry.status ?? null,
    emailId:
      entry.channel === "email" ? (entry.first_activity_id ?? null) : null,
  };
}

function folded(view: Contact360, t: Translator, recordZone: string): Said[] {
  // A meeting still to come belongs on the Next bar, not among what was said.
  const now = Date.now();
  const rows = (view.activities?.data ?? []).filter(
    (row) => row.kind !== "task" && activityHasHappened(row, now, recordZone),
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
      direction: row.direction,
      occurredAt: row.occurred_at,
      occurredPrecision: row.occurred_precision,
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

// Only an answer on this conversation settles it; a contact can owe several.
function statusOf(row: Activity, rows: readonly Activity[]): Status {
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

const DIRECTION: Readonly<
  Record<"inbound" | "outbound", Partial<Record<Activity["kind"], MessageKey>>>
> = {
  inbound: {
    email: "contact.card.theyWrote",
    message: "contact.card.theyWrote",
    call: "contact.card.theyCalled",
  },
  outbound: {
    email: "contact.card.youWrote",
    message: "contact.card.youWrote",
    call: "contact.card.youCalled",
  },
};

// Who spoke. A meeting is both sides by its nature; a note has no side.
function directionWord(
  kind: Activity["kind"],
  direction: Said["direction"],
  t: Translator,
): string {
  if (kind === "meeting") {
    return t("contact.card.bothSides");
  }
  if (direction !== "inbound" && direction !== "outbound") {
    return "";
  }
  const key = DIRECTION[direction][kind];
  return key ? t(key) : "";
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
