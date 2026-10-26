import { Lock, Send } from "lucide-react";
import { useState } from "react";
import { formatNumber } from "../format/format";
import { type Locale, translatePlural, useLocale, useT } from "../i18n";
import { Avatar, Button } from "./atoms";
import { attachmentCount, EmailFiles, EmailWords } from "./emailentry";
import { Eyebrow } from "./eyebrow";
import { type ContactOn, withWhom } from "./participants";
import { ProvenanceTag } from "./trust";
import { type Visibility, VisibilityBadge } from "./visibility";
import "./composed.css";

import {
  automaticLabel,
  CommitmentReviewFlag,
  directionClass,
  entryDay,
  entryMove,
  entryTimeOfDay,
  groupCountLabel,
  MoveFlag,
  TIMELINE_ICON,
  TIMELINE_KIND_LABEL,
  type TimelineEntry,
  type TimelineGroup,
  TimelineWhen,
} from "./timelineentry";
import { TimelineText } from "./timelinetext";

// How many of a thread's messages stand open before the rest fold behind a
// count. Three is a conversation a reader takes in at a glance: the ask,
// the answer and the follow-up. A twelve-message thread drawn whole would
// push every other event on the record below the fold.
const THREAD_OPEN_MEMBERS = 3;

// Who was on the other side of a message, for the row's lead line and the
// thread's participant list. The server's own counterparty comes first (it is
// the row model EmailEntry draws), then the resolved link names. Never for a
// withheld row: the name beside a message a reader may not open is the thing
// the audience limited. EmailEntry and TimelineRow both drop it there.
function otherSideOf(entry: TimelineEntry): string | undefined {
  if (entry.withheld) {
    return undefined;
  }
  return entry.emailSummary?.counterparty?.trim() || entry.counterparts;
}

// The server's summary of every member that has one, for the files the whole
// conversation carried. A chat member has none and adds nothing.
function memberSummaries(
  entries: readonly TimelineEntry[],
): NonNullable<TimelineEntry["emailSummary"]>[] {
  return entries.flatMap((entry) =>
    entry.emailSummary ? [entry.emailSummary] : [],
  );
}

// The same contacts one at a time, for a set and for a face. The adapter's
// when it had any; otherwise the one phrase the row shows, the best name there
// is and its own key. Nothing on a withheld row, as above.
function otherSide(entry: TimelineEntry): readonly ContactOn[] {
  if (entry.withheld) {
    return [];
  }
  if (entry.counterpartContacts?.length) {
    return entry.counterpartContacts;
  }
  const who = otherSideOf(entry);
  return who ? [{ key: who, name: who }] : [];
}

// Who a thread was with, as one phrase: the other side's names, each once,
// joined once at the end. Joining per message and then collecting the
// phrases listed one contact under two spellings. Our own seats are not
// listed. Every thread on this record is with us, and a name that appears
// on all of them tells a reader nothing.
function threadParticipants(
  entries: readonly TimelineEntry[],
  t: ReturnType<typeof useT>,
  locale: Locale,
): string | undefined {
  const names = new Set<string>();
  for (const entry of entries) {
    for (const contact of otherSide(entry)) {
      names.add(contact.name);
    }
  }
  return withWhom([...names], t, locale);
}

// The lead of a message in a thread: who, and what they did. An inbound
// message is their word and carries their name. An outbound one is ours, and
// says "we" unless the reader logged it themselves. A captured mail names
// the other side, not which seat sent it. So "you" on a colleague's mail
// would be a claim the row cannot make.
function messageLead(
  entry: TimelineEntry,
  t: ReturnType<typeof useT>,
): { actor: string; verb?: string } {
  const automatic = automaticLabel(entry);
  if (automatic) {
    return { actor: t(automatic) };
  }
  const who = otherSideOf(entry);
  if (entry.direction === "inbound") {
    return {
      actor: who ?? t("timeline.thread.them"),
      verb: t("timeline.thread.wrote"),
    };
  }
  if (entry.direction === "outbound") {
    // An imported row knows who sent it, and neither "we" nor "you" is that
    // colleague. The import ran as one administrator, so every row it wrote
    // reads as that seat's own sending. The author is asked first for the same
    // reason the provenance tag asks it first.
    const author =
      entry.provenance.kind === "human"
        ? entry.provenance.author?.display_name
        : undefined;
    const self = entry.provenance.kind === "human" && entry.provenance.self;
    return {
      actor:
        author ?? (self ? t("timeline.thread.you") : t("timeline.thread.we")),
      verb: who
        ? t("timeline.thread.sentTo", { who })
        : t("timeline.thread.sent"),
    };
  }
  // No direction recorded: the row names who it was with, or failing that
  // what it was, and claims nothing about who spoke.
  return { actor: who ?? t(TIMELINE_KIND_LABEL[entry.kind]) };
}

// Who may read a message in a thread, when that is worth a mark: sealed rows
// and withheld ones. The open default (`team` on a mail, `workspace` on
// anything else) draws nothing. The reason is the one TimelineRow gives: a
// mark on every open row is decoration a reader learns to skip.
//
// Answers the state; the badge at the mount below draws it. Reading a
// message's access word is not drawing the message. Keeping the two apart
// lets that be seen rather than argued: there is no markup here to check.
function messageVisibilityState(entry: TimelineEntry): Visibility | null {
  if (entry.withheld) {
    return "withheld";
  }
  const status = entry.emailSummary?.display_status;
  if (status && status !== "team") {
    return status;
  }
  if (entry.audience && entry.audience !== "workspace") {
    return entry.audience;
  }
  return null;
}

// Whether a thread's member can be opened, and with what. Only a message drawn
// from the server's summary opens, because its words are one span. A folded
// body carries controls of its own that cannot sit inside a button.
//
// Its own function on the same terms as messageVisibilityState above. Deciding
// that an entry has a message to open reads the summary and draws no part of
// it.
function threadMessageOpener(entry: TimelineEntry): (() => void) | undefined {
  return entry.emailSummary ? entry.onOpenEmail : undefined;
}

// The mark beside a message: the sender's face on their word, a send mark on
// ours, a lock on one the reader may not open. The kind's own icon goes where
// nobody is named. A monogram of "We" or of "Them" would be a face nobody has.
function MessageMark({ entry }: Readonly<{ entry: TimelineEntry }>) {
  if (entry.withheld) {
    return (
      <span className="tl-msg-mark">
        <Lock aria-hidden />
      </span>
    );
  }
  // The face is one contact's, the first on the other side. It is keyed on its
  // record so it is that contact's colour everywhere, never the lead line's
  // phrase.
  const [face] = otherSide(entry);
  if (entry.direction === "inbound" && face) {
    return <Avatar name={face.name} identity={face.key} />;
  }
  const Icon =
    entry.direction === "outbound" ? Send : TIMELINE_ICON[entry.kind];
  return (
    <span className="tl-msg-mark">
      <Icon aria-hidden />
    </span>
  );
}

// What a message in a thread says. A mail with the server's summary draws it
// through EmailWords, the canonical spelling of a message's words. That is
// also where the withheld rule lives, so this row cannot answer that question
// differently from the row beside it. A message without a summary (a chat, or
// a server that has not caught up) draws its body through TimelineText. That
// is the one the chronicle uses, with the same fold. A withheld row draws the
// sentence where its words would be, whatever it is handed.
function MessageWords({
  entry,
  t,
}: Readonly<{ entry: TimelineEntry; t: ReturnType<typeof useT> }>) {
  if (entry.withheld) {
    return <span className="tl-withheld">{t("timeline.withheld")}</span>;
  }
  if (entry.emailSummary) {
    return <EmailWords summary={entry.emailSummary} />;
  }
  return entry.body ? <TimelineText text={entry.body} entry={entry} /> : null;
}

/**
 * ThreadMessage is one message inside a thread's card: who wrote it, what it
 * said, and when. It is not TimelineRow. A member of a conversation has no
 * date gutter, no rail and no kind of its own to announce. The card has
 * already placed the conversation on the axis and said what it is.
 *
 * Openable when, and only when, the surface mounted a drawer for it. It uses
 * the same opener the canonical row takes. Read here but not openable, its
 * messages would be a different kind of object from the mail they are. Only
 * a message drawn from the server's summary opens, because its words are one
 * span. A folded body carries controls of its own that cannot sit inside a
 * button.
 */
function ThreadMessage({
  entry,
  zone,
  cardDay,
}: Readonly<{
  entry: TimelineEntry;
  zone: string;
  // The day the card's gutter already prints: the newest member's. A member
  // from that day says only its time; one from another day says its day too.
  cardDay: string;
}>) {
  const { locale } = useLocale();
  const t = useT();
  const lead = messageLead(entry, t);
  const visibility = messageVisibilityState(entry);
  const onOpen = threadMessageOpener(entry);
  const day = entryDay(entry, locale, zone);
  const content = (
    <>
      <MessageMark entry={entry} />
      <span className="tl-msg-body">
        <span className="tl-msg-lead">
          <b className="tl-msg-who">{lead.actor}</b>
          {lead.verb && <span>{lead.verb}</span>}
          {visibility && <VisibilityBadge state={visibility} />}
          {entry.emailSummary && (
            <EmailFiles summaries={[entry.emailSummary]} />
          )}
          <CommitmentReviewFlag
            count={entry.withheld ? 0 : entry.commitmentReviewCount}
          />
        </span>
        <MessageWords entry={entry} t={t} />
      </span>
      {/* The time, and the day only when it is not the one the gutter already
          shows: a one-message thread printed its date twice a hand apart, and
          an older member from another day still needs its own. */}
      <span className="tl-msg-when t-num">
        {[day !== cardDay && day, entryTimeOfDay(entry, locale, zone)]
          .filter(Boolean)
          .join(" ")}
      </span>
    </>
  );
  if (!onOpen) {
    return <div className="tl-msg">{content}</div>;
  }
  return (
    <button
      type="button"
      className="tl-msg tl-msg-open"
      onClick={onOpen}
      aria-haspopup="dialog"
    >
      {content}
    </button>
  );
}

/**
 * ThreadRow is a conversation on the chronology, drawn open. One card sits on
 * the body side of the rail. It carries what the thread is (the kind, how many
 * messages, who it was with, whose move it is) over its subject. Under that
 * come the messages themselves, newest first.
 *
 * Open rather than folded behind a summary of its newest message. A thread is
 * the thing a reader came to the history to read: the ask, the answer and
 * where it stands. All of it is in one place, without opening each message in
 * turn. Past the first few the rest fold behind a count. A long exchange
 * then does not push the rest of the record's story off the screen.
 */
export function ThreadRow({
  group,
  zone,
  onOpenThread,
}: Readonly<{
  group: TimelineGroup;
  zone: string;
  onOpenThread?: (threadKey: string) => void;
}>) {
  const { locale } = useLocale();
  const t = useT();
  const [allOpen, setAllOpen] = useState(false);
  const [newest] = group.entries;
  if (newest === undefined) {
    return null;
  }
  const Icon = TIMELINE_ICON[newest.kind];
  const threadKey = newest.threadKey;
  const participants = threadParticipants(group.entries, t, locale);
  const folded = group.entries.length - THREAD_OPEN_MEMBERS;
  const shown = allOpen
    ? group.entries
    : group.entries.slice(0, THREAD_OPEN_MEMBERS);
  // The card's total is for files the messages in view do not already say.
  // When one message carries them all and it is on screen, its own mark is the
  // total. A second one would count the same files twice.
  const carriers = group.entries.filter(
    (entry) => entry.emailSummary && attachmentCount(entry.emailSummary) > 0,
  );
  const [carrier] = carriers;
  const filesAreOnOneShownMessage =
    carriers.length === 1 && carrier !== undefined && shown.includes(carrier);
  // The subject, from a member whose words the reader may read. A withheld
  // member's title is the kind it was, not the subject it had. A thread
  // every member of which is withheld says so where its subject would go.
  const subject = group.entries.find((entry) => !entry.withheld)?.title;
  return (
    <li className={directionClass(newest.direction)}>
      {/* The same three columns a single row sits on (date, rail, what
          happened), because a conversation is a row of the chronology. Its
          mark is the kind's own icon: a dot would say it was one message. */}
      <TimelineWhen entry={newest} zone={zone} />
      <span className="tl-rail" aria-hidden="true">
        <span className="tl-icon">
          <Icon aria-hidden />
        </span>
      </span>
      <div className="tl-body">
        <div className="tl-thread">
          <span className="tl-head">
            <Eyebrow>{t("timeline.group.kind")}</Eyebrow>
            <span>{groupCountLabel(group, locale)}</span>
            {participants && <span>{participants}</span>}
            {/* Whose move the conversation waits on, read off its newest
                message. It goes on the card that stands for it, never on the
                members inside. */}
            {entryMove(newest) && <MoveFlag entry={newest} />}
            <CommitmentReviewFlag
              count={group.entries.reduce(
                (count, entry) =>
                  count +
                  (entry.withheld ? 0 : (entry.commitmentReviewCount ?? 0)),
                0,
              )}
            />
            {/* The newest member's verbs stand for the conversation: Reply
                answers it, and Relink on it offers to move the rest of the
                thread. */}
            {newest.actions && (
              <span className="tl-thread-actions">{newest.actions}</span>
            )}
          </span>
          {subject ? (
            <span className="tl-title">{subject}</span>
          ) : (
            <span className="tl-title tl-withheld">
              <Lock aria-hidden />
              {t("timeline.withheld")}
            </span>
          )}
          <ul className="tl-thread-messages">
            {shown.map((entry) => (
              <li key={entry.id}>
                <ThreadMessage
                  entry={entry}
                  zone={zone}
                  cardDay={entryDay(newest, locale, zone)}
                />
              </li>
            ))}
          </ul>
          <span className="tl-meta">
            {folded > 0 && (
              <Button
                aria-expanded={allOpen}
                onClick={() => setAllOpen(!allOpen)}
              >
                {allOpen
                  ? t("timeline.group.hideEarlier")
                  : translatePlural(locale, "timeline.group.earlier", folded, {
                      count: formatNumber(folded, locale),
                    })}
              </Button>
            )}
            {!filesAreOnOneShownMessage && (
              <EmailFiles summaries={memberSummaries(group.entries)} />
            )}
            <ProvenanceTag provenance={newest.provenance} />
            {/* A thread cut by the page edge can be completed when the page
                passed a handler; where it cannot, the notice still stands,
                because rendering neither would present the cut as the whole
                of it. */}
            {group.partial &&
              (threadKey && onOpenThread ? (
                <Button onClick={() => onOpenThread(threadKey)}>
                  {t("timeline.group.openThread")}
                </Button>
              ) : (
                <span className="t-caption">
                  {t("timeline.group.mayContinue")}
                </span>
              ))}
          </span>
        </div>
      </div>
    </li>
  );
}
