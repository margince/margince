import {
  CalendarClock,
  CheckSquare,
  Mail,
  MessageCircle,
  PencilLine,
  Phone,
  StickyNote,
} from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import {
  activityDay,
  activityTimeOfDay,
  type OccurredPrecision,
} from "../format/activitywhen";
import { formatNumber } from "../format/format";
import {
  type Locale,
  translatePlural,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { MessageKey } from "../i18n/en";
import { Badge } from "./atoms";
import { moveLabel } from "./emailentry";
import type { ContactOn } from "./participants";
import type { Provenance } from "./trust";
import "./composed.css";

/**
 * TimelineGroup is a run of entries the reader sees as one event: a
 * conversation, or one message sent to several contacts. It lives here with the
 * component that renders it. The rules that build one are a screen concern,
 * but the shape is the list's own vocabulary.
 */
export type TimelineGroup = {
  /** The newest member's id; the list keys on it. */
  id: string;
  kind: "thread" | "bulk" | "single";
  /** Newest first, like the list itself. */
  entries: TimelineEntry[];
  /** This group may continue past what the page holds. */
  partial: boolean;
};

type EmailSummary = components["schemas"]["EmailSummary"];

type ActivityAudience = components["schemas"]["ActivityAudience"];

export type TimelineEntry = {
  id: string;
  // The backend's activity kinds, not a reduced set: collapsing call, task
  // and the chat kinds into "note" told the reader an email was a note.
  //
  // `change` is not an activity: it is a field edit projected from the audit
  // spine. It rides the same list because what was said to an account and
  // what was changed about it are one chronology to the reader. Kept apart, a
  // rep comparing "we told them X" against "someone set stage to Y" had to
  // hold two orderings in their head.
  kind: "email" | "meeting" | "note" | "call" | "task" | "message" | "change";
  title: string;
  isDone?: boolean;
  atIso: string;
  // `day` when only the date is known, so the row draws no time.
  atPrecision: OccurredPrecision;
  provenance: Provenance;
  commitmentReviewCount?: number;
  // A right-aligned per-row action slot (Reply / Relink). Absent on rows with
  // no affordance, which then draw no action slot.
  actions?: ReactNode;
  // The records this entry is about, as the backend's links[] reports them.
  // They are already pruned to what the reader may see.
  via?: ReactNode;
  /**
   * Which way it went, when the record knows: `outbound` is us reaching out,
   * `inbound` is them coming back.
   *
   * A single undifferentiated stream reads as "things happened here". It hides
   * the one shape a rep looks for before they reach out: whether the last few
   * moves were all ours. Absent on kinds that have no direction (a note, a
   * task), which render with no direction mark.
   */
  direction?: "inbound" | "outbound" | null;
  /**
   * Who was on the other end, already resolved to names by the caller.
   *
   * "We sent" answers half a question. The half a reader came for is who it
   * went to. An account with four contacts has four meanings of "we sent". A
   * row that does not say which one is a row they have to open.
   */
  counterparts?: string;
  /**
   * The same contacts, key and name each, before they were joined into the
   * phrase above. A thread lists everyone it was with and draws each sender's
   * face, and both need a contact. A set of phrases counts "Ida Keller" and
   * "Ida Keller, Marc Dubois" as two, and a monogram of a phrase is nobody's.
   * With no name resolved, the server's phrase on the record it names.
   */
  counterpartContacts?: readonly ContactOn[];
  /**
   * The server's own row model for an email, present when and only when
   * `kind` is `email`. It is what EmailEntry draws. So the timeline hands the
   * canonical row its data rather than re-deriving a reading of the message
   * here. The canonical row exists to replace four independent readings, and
   * this component was one of them.
   */
  emailSummary?: EmailSummary;
  /**
   * Opens the canonical detail for this message. The caller owns it, because
   * the screen mounts the drawer, not the list. Absent leaves the row readable
   * and not openable, which is what a surface with nowhere to open it should
   * draw.
   */
  onOpenEmail?: () => void;
  /**
   * What kind of thing happened to the record, in the words a reader uses:
   * "field updated", "completed". It sits where an exchange's direction sits,
   * because it answers the same question. The badge says what sort of entry
   * this is, and this says what it did.
   */
  qualifier?: string;
  /**
   * The provider's own conversation id, when capture stamped one. It is what
   * makes a thread a thread. A subject match would merge two unrelated
   * "Re: Update" exchanges and split one renamed mid-conversation.
   */
  threadKey?: string | null;
  /**
   * The sender declared this message bulk (RFC 2369 List-Unsubscribe). Per
   * message, never per sender: the same address sends a newsletter and a reply.
   */
  bulkAttested?: boolean;
  /**
   * The message's own subject, when it had one. This is not the rendered
   * `title`, which falls back to the body (or to the kind) so a subjectless
   * row still has something to show. Bulk grouping keys on this. Keyed on the
   * title, two subjectless messages that happen to render the same text would
   * fold into one summary and hide each other.
   */
  subject?: string | null;
  /**
   * What the message actually said.
   *
   * A timeline of subject lines is a list of things you cannot read. The rep
   * knows an email happened, and still has to leave for their mail client to
   * find out what was in it. The body rides along in the same composite read
   * the row came from, so showing it costs nothing.
   *
   * Absent on a row whose body was erased under retention or an Art. 17
   * request. That is why this is optional rather than empty string.
   */
  body?: string | null;
  /** A pasted or uploaded transcript, drawn as its lines rather than markdown. */
  transcript?: boolean;
  /**
   * The row is discoverable but its content is not this reader's. A human
   * limited the activity's audience and the reader is not in it. The row keeps
   * its place (date, direction, kind) and says so. It does not vanish into
   * what reads as silence (the withheld state of README § "Absent, disabled,
   * or withheld").
   */
  withheld?: boolean;
  /**
   * Who may read this row's content, for a row that is not an email. An email
   * carries it inside `emailSummary` and the canonical row draws it. A note or
   * a logged call limited by a human carried it nowhere. So the one row on a
   * timeline a reader most needed to recognise as sealed looked like every
   * other. `workspace` draws nothing: it is the default, and a mark on every
   * open row is decoration a reader learns to skip.
   */
  audience?: ActivityAudience | null;
  /**
   * Rendered content for a row whose substance is not prose: the old→new
   * diff on a `change` row. Sits where the body would, so a change reads at
   * the same place in the row as a message does.
   */
  detail?: ReactNode;
};

// One icon per kind and one for all messaging transports. An extension unit's
// transport has no entry here, so the row names it in the label.
// The kind in a word beside the row, not an icon alone. A glyph is a thing to
// recognise and a word is a thing to read. A timeline that mixes mail,
// meetings and field edits is scanned by reading.
export const TIMELINE_KIND_LABEL = {
  email: "timeline.kind.email",
  meeting: "timeline.kind.meeting",
  note: "timeline.kind.note",
  call: "timeline.kind.call",
  task: "timeline.kind.task",
  message: "timeline.kind.message",
  change: "timeline.kind.change",
} as const satisfies Record<TimelineEntry["kind"], MessageKey>;

/**
 * The mark on the axis: solid for something that was said, hollow for
 * something that merely changed.
 *
 * A reader following a conversation can then track the solid dots past the
 * field edits between them without reading either. That is why the two kinds
 * share one column instead of two tabs.
 */
// Indigo marks a change the agent made: the rail says who wrote before a
// reader reads a word of it.
export function dotClass(entry: TimelineEntry): string {
  if (entry.kind !== "change") {
    return "tl-dot";
  }
  return entry.provenance.kind === "agent"
    ? "tl-dot tl-dot-agent"
    : "tl-dot tl-dot-quiet";
}

export const TIMELINE_ICON = {
  email: Mail,
  meeting: CalendarClock,
  note: StickyNote,
  call: Phone,
  task: CheckSquare,
  message: MessageCircle,
  change: PencilLine,
} as const;

// A link inside a captured message, rendered as an element rather than as
// markup: the body is escaped text and stays that way. The visible label is
// the URL itself, so the destination a reader checks is the destination they
// get. A mail we did not write is not a place to render a friendly label over
// a different address.

// directionClass tracks the row to one side of the spine: ours or theirs.
export function directionClass(direction: TimelineEntry["direction"]): string {
  if (direction === "outbound") {
    return "tl-out";
  }
  return direction === "inbound" ? "tl-in" : "";
}

export function noteClass(entry: TimelineEntry): string {
  return entry.kind === "note" ? "tl-note" : "";
}

// A conversation is an exchange somebody can answer: mail, or a message on a
// connected transport. Calls, meetings and notes are events and asides. There
// is one spelling: the Conversations cut and the whose-move flag both ask it.
// Two copies would let a new transport join one answer and not the other.
const CONVERSATION_KINDS: ReadonlySet<string> = new Set(["email", "message"]);

export function isConversationKind(kind: string): boolean {
  return CONVERSATION_KINDS.has(kind);
}

/**
 * MoveFlag is whose move a conversation waits on: the server's move for its
 * newest message (moveLabel). A colleague's inbound invitation owes nobody a
 * reply, so the direction cannot say it. Only on the row that stands for the
 * conversation, never on the members of an expanded thread.
 */
export function entryMove(entry: TimelineEntry): MessageKey | undefined {
  return moveLabel(entry.emailSummary, entry.withheld === true);
}

export function MoveFlag({ entry }: Readonly<{ entry: TimelineEntry }>) {
  const t = useT();
  const move = entryMove(entry);
  if (!move) {
    return null;
  }
  return (
    <Badge
      tone={entry.emailSummary?.move === "needs_reply" ? "warning" : undefined}
    >
      {t(move)}
    </Badge>
  );
}

/**
 * TimelineWhen is the row's place on the axis: the day, and under it the time
 * of day when one is known. The day alone told a reader two calls happened on
 * the 26th, and not which came first. Nor did it say whether the reply landed
 * an hour after the ask or a working day later. That is the one thing a
 * chronology is opened to settle. Tabular digits keep the column straight
 * whatever each date's digits are.
 */
export function TimelineWhen({
  entry,
  zone,
}: Readonly<{ entry: TimelineMoment; zone: string }>) {
  const { locale } = useLocale();
  const time = entryTimeOfDay(entry, locale, zone);
  return (
    <span className="tl-when t-num">
      {entryDay(entry, locale, zone)}
      {time && <span className="tl-when-time">{time}</span>}
    </span>
  );
}

type TimelineMoment = Pick<TimelineEntry, "atIso" | "atPrecision">;

/** The date an entry falls on. */
export function entryDay(
  entry: TimelineMoment,
  locale: Locale,
  zone: string,
): string {
  return activityDay(entry.atIso, entry.atPrecision, locale, zone);
}

/** An entry's clock reading, or undefined when only its day is known. */
export function entryTimeOfDay(
  entry: TimelineMoment,
  locale: Locale,
  zone: string,
): string | undefined {
  return (
    activityTimeOfDay(entry.atIso, entry.atPrecision, locale, zone) ?? undefined
  );
}

// groupCountLabel counts the group's members in words that read. Both group
// kinds can hold one member. A thread can have its other messages on another
// page, and a sender can attest a single message as a bulk send. The plural
// forms rendered "1 messages" and "sent to 1 contacts" for it.
export function groupCountLabel(group: TimelineGroup, locale: Locale): string {
  const count = group.entries.length;
  const base =
    group.kind === "bulk" ? "timeline.group.bulk" : "timeline.group.thread";
  return translatePlural(locale, base, count, {
    count: formatNumber(count, locale),
  });
}

export function CommitmentReviewFlag({
  count = 0,
}: Readonly<{ count?: number }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  return count > 0 ? (
    <Badge tone="ai">
      {plural("commitment.review.count", count, {
        count: formatNumber(count, locale),
      })}
    </Badge>
  ) : null;
}

export function TaskStateFlag({ entry }: Readonly<{ entry: TimelineEntry }>) {
  const t = useT();
  if (entry.kind !== "task" || entry.withheld || entry.isDone === undefined)
    return null;
  return (
    <Badge tone={entry.isDone ? "success" : "default"}>
      {t(entry.isDone ? "tasks.isDone" : "tasks.isOpen")}
    </Badge>
  );
}
