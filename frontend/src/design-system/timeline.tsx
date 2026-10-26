import { Lock } from "lucide-react";
import type { ReactNode } from "react";
import { useState } from "react";
import { useLocale, useT } from "../i18n";
import { Button } from "./atoms";
import { EmailEntry } from "./emailentry";
import { Eyebrow } from "./eyebrow";
import { ProvenanceTag } from "./trust";
import { VisibilityBadge } from "./visibility";
import "./composed.css";

import {
  AutomaticFlag,
  CommitmentReviewFlag,
  directionClass,
  dotClass,
  entryTimeOfDay,
  groupCountLabel,
  MoveFlag,
  noteClass,
  TaskStateFlag,
  TIMELINE_ICON,
  TIMELINE_KIND_LABEL,
  type TimelineEntry,
  type TimelineGroup,
  TimelineWhen,
} from "./timelineentry";
import { TimelineText } from "./timelinetext";
import { ThreadRow } from "./timelinethread";

export function TimelineList({
  entries,
  zone,
}: Readonly<{ entries: TimelineEntry[]; zone: string }>) {
  return (
    <ul className="timeline">
      {entries.map((entry) => (
        <TimelineRow key={entry.id} entry={entry} zone={zone} />
      ))}
    </ul>
  );
}

/**
 * GroupedTimelineList renders conversations rather than messages.
 *
 * A thread is one card, open. It states what it is ("3 messages", who with,
 * whose move) over its subject, then shows the messages themselves.
 * A bulk send is folded and says "sent to 3 contacts" before what it says.
 * The reader scans for an event there, not for a sentence. Expanding it shows
 * the rows the flat list would show, from the same component, so the two
 * cannot drift.
 *
 * A group that may continue past the page says so. A summary that implied it
 * was whole would be a worse answer than the repetition this replaced.
 */
export function GroupedTimelineList({
  groups,
  zone,
  onOpenThread,
}: Readonly<{
  groups: readonly TimelineGroup[];
  zone: string;
  // Fetches the rest of a conversation the page holds only part of. Absent for
  // a caller that cannot complete it: a bulk group has no thread to ask for.
  onOpenThread?: (threadKey: string) => void;
}>) {
  return (
    <ul className="timeline">
      {groups.map((group) =>
        group.kind === "single" ? (
          group.entries[0] && (
            <TimelineRow
              key={group.id}
              entry={group.entries[0]}
              zone={zone}
              flag={<MoveFlag entry={group.entries[0]} />}
            />
          )
        ) : group.kind === "thread" ? (
          <ThreadRow
            key={group.id}
            group={group}
            zone={zone}
            onOpenThread={onOpenThread}
          />
        ) : (
          <BulkGroupRow key={group.id} group={group} zone={zone} />
        ),
      )}
    </ul>
  );
}

// BulkGroupRow is one send to several contacts, folded. While closed, the
// newest copy stands for the send. Opening it lists every copy through the
// ordinary row. A thread is not drawn here: it is a conversation, and
// ThreadRow draws it open, as one card of messages.
function BulkGroupRow({
  group,
  zone,
}: Readonly<{
  group: TimelineGroup;
  zone: string;
}>) {
  const { locale } = useLocale();
  const t = useT();
  const [open, setOpen] = useState(false);
  const [newest] = group.entries;
  if (newest === undefined) {
    return null;
  }
  const Icon = TIMELINE_ICON[newest.kind];
  return (
    <li className={directionClass(newest.direction)}>
      {/* A group sits on the same three columns a single row does (date,
          rail, what happened), because it is a row of the chronology. One
          that stepped out of the axis read as a different list wedged into
          this one. Its mark is the kind's own icon: the whole conversation is
          one thing that happened, and a dot would say it was one message. */}
      <TimelineWhen entry={newest} zone={zone} />
      <span className="tl-rail" aria-hidden="true">
        <span className="tl-icon">
          <Icon aria-hidden />
        </span>
      </span>
      <div className="tl-body">
        {/* What the send is, while it is closed. No whose-move flag: a bulk
            group is one outbound send with no reply expected, so it carries
            no claim.

            The newest copy stands for the send, and that copy is drawn by
            the SAME component a lone one is — subject, who, the preview the
            server composed, the access badge. Writing the summary by hand
            here was the defect: a company timeline of grouped sends showed
            old-style rows beside canonical ones on the same page, because a
            group never reached EmailEntry. Expanded, the members draw
            themselves below and a summary above them would say the newest
            one twice. */}
        {open ? (
          <span className="tl-title">{newest.title}</span>
        ) : newest.kind === "email" && newest.emailSummary ? (
          <EmailEntry
            summary={newest.emailSummary}
            timestamp={entryTimeOfDay(newest, locale, zone)}
            onOpen={newest.onOpenEmail}
            // Absent on the reply composer, whose rows are the thread being
            // answered rather than a place to act from. Every other surface
            // that draws this timeline mounts a drawer and passes an opener.
            whyNotOpenable="noReader"
          />
        ) : (
          <>
            <span className="tl-title">{newest.title}</span>
            {/* Never for a withheld entry: a summary row must not show a
                reader words the row itself refuses. */}
            {newest.body && !newest.withheld && (
              <TimelineText text={newest.body} entry={newest} />
            )}
          </>
        )}
        <span className="tl-meta">
          <span>{groupCountLabel(group, locale)}</span>
          <ProvenanceTag provenance={newest.provenance} />
          <Button aria-expanded={open} onClick={() => setOpen(!open)}>
            {open ? t("timeline.group.collapse") : t("timeline.group.expand")}
          </Button>
          {/* A bulk send cannot be completed: it has no thread to ask the
              server for. The notice still stands, because rendering nothing
              would present a group cut off by the page edge as the whole of
              it. */}
          {group.partial && (
            <span className="t-caption">{t("timeline.group.mayContinue")}</span>
          )}
        </span>
        {open && (
          <ul className="timeline tl-group-members">
            {group.entries.map((entry) => (
              <TimelineRow key={entry.id} entry={entry} zone={zone} />
            ))}
          </ul>
        )}
        {/* The newest copy's verbs stand for the send: Relink on it offers to
            move the rest, so a mis-filed send is fixed from its summary row
            without opening it first. */}
        {newest.actions && <span className="tl-actions">{newest.actions}</span>}
      </div>
    </li>
  );
}

// Which way it went, and with whom.
//
// The name is dropped, not guessed at, when nothing resolved it. A row that
// says "Sent to" and stops loses the reader's trust more than one that only
// says "Sent".
function directionPhrase(
  entry: TimelineEntry,
  t: ReturnType<typeof useT>,
): string {
  const who = entry.counterparts;
  if (!who) {
    return entry.direction === "outbound"
      ? t("timeline.sent")
      : t("timeline.received");
  }
  if (entry.direction === "outbound") {
    return t("timeline.sentTo", { who });
  }
  if (entry.direction === "inbound") {
    return t("timeline.receivedFrom", { who });
  }
  // A meeting or a note has no side. It still has contacts in it, and naming
  // them is the whole reason this line exists.
  return t("timeline.withWhom", { who });
}

/**
 * TimelineRow is one entry. It is split out of the list so a grouped view can
 * render the same row inside an expanded conversation. A second rendering of
 * a message would drift from this one the first time either changed.
 */
export function TimelineRow({
  entry,
  zone,
  flag,
}: Readonly<{
  entry: TimelineEntry;
  zone: string;
  // A chip qualifying the whole exchange this row stands for: the whose-move
  // flag on a lone message. The grouped list passes it, not this row. The
  // same row, drawn as a member of an expanded thread, must not carry a
  // verdict about the conversation it is one reply in.
  flag?: ReactNode;
}>) {
  const { locale } = useLocale();
  const t = useT();
  // A note is the rep's own words about the record, not an exchange with it.
  // The row says so: its body sits on a plate of its own instead of running
  // in the message rhythm around it.
  const rowClass = [directionClass(entry.direction), noteClass(entry)]
    .filter(Boolean)
    .join(" ");
  // An email draws the canonical row, the one reading of a message in the
  // product. It keeps the date gutter and the rail, which place the row on the
  // chronology and belong to the timeline. EmailEntry draws everything the
  // message itself says.
  //
  // Every other kind keeps the row below. A note is the rep's own words, a
  // change is a field edit, a meeting has a transcript. None of them has an
  // email's shape. Giving them one would repeat the collapse of rendering a
  // call as a note.
  if (entry.kind === "email" && entry.emailSummary) {
    return (
      <li className={rowClass}>
        <TimelineWhen entry={entry} zone={zone} />
        <span className="tl-rail" aria-hidden="true">
          <span className={dotClass(entry)} />
        </span>
        <div className="tl-body">
          {/* The TIME, not the date: the gutter beside this already carries
              the day, and printing it twice on one row spends the space that
              tells two messages on one subject apart. */}
          <EmailEntry
            summary={entry.emailSummary}
            timestamp={entryTimeOfDay(entry, locale, zone)}
            onOpen={entry.onOpenEmail}
            // As above: absent on the reply composer and nowhere else.
            whyNotOpenable="noReader"
          />
          <AutomaticFlag entry={entry} />
          <CommitmentReviewFlag
            count={entry.withheld ? 0 : entry.commitmentReviewCount}
          />
          {flag}
          {entry.actions && <span className="tl-actions">{entry.actions}</span>}
        </div>
      </li>
    );
  }
  return (
    <li className={rowClass}>
      {/* The date leads the row, in its own gutter. A chronology is read down
          the dates — a reader looking for "what happened in August" scans one
          column rather than the end of every line. */}
      <TimelineWhen entry={entry} zone={zone} />
      {/* The axis, and this row's place on it. The rail runs THROUGH the row
          rather than a rule sitting under it: a chronology is one thread, and a
          border per entry drew it as a stack of unrelated cards. Filled for
          something that was SAID, hollow for something that merely changed, so
          a reader tracking a conversation follows the solid dots straight past
          the field edits between them. */}
      <span className="tl-rail" aria-hidden="true">
        <span className={dotClass(entry)} />
      </span>
      {/* A div, not a span: a change row's detail is a field diff whose
                long-value side is a focusable region — flow content, invalid
                inside phrasing content. */}
      <div className="tl-body">
        {/* What KIND of thing this was, as a kicker and not a status, and
            which way it went — one line above the headline, because both
            qualify it and set inline they read as the subject's first words. */}
        <span className="tl-head">
          <Eyebrow>{t(TIMELINE_KIND_LABEL[entry.kind])}</Eyebrow>
          <TaskStateFlag entry={entry} />
          {/* What the record DID, for a row that is not an exchange: the kind
              says this is a record entry, and this says what happened to it. */}
          {entry.qualifier && <span>{entry.qualifier}</span>}
          {/* Which way it went and who was at the other end, as one phrase.
              The direction alone is a fact about us; with the name it is a
              fact about the relationship, which is what the row is for.
              A WITHHELD row keeps the direction and loses the name: "Received
              from Ana" beside a message whose subject the row just refused
              still says who this record is talking to, which is the thing the
              audience limited. */}
          {(entry.direction || entry.counterparts) && (
            <span>
              {directionPhrase(
                entry.withheld ? { ...entry, counterparts: undefined } : entry,
                t,
              )}
            </span>
          )}
          {entry.via}
          {/* Who may read it, when that is worth a mark: sealed rows and
              withheld ones. Withheld first, because a held row's `audience`
              is not the reader's to see and the mark must not claim one. */}
          {entry.withheld ? (
            <VisibilityBadge state="withheld" />
          ) : (
            entry.audience &&
            entry.audience !== "workspace" && (
              <VisibilityBadge state={entry.audience} />
            )
          )}
          <AutomaticFlag entry={entry} />
          <CommitmentReviewFlag
            count={entry.withheld ? 0 : entry.commitmentReviewCount}
          />
          {flag}
        </span>
        {entry.withheld ? (
          <span className="tl-title tl-withheld">
            <Lock aria-hidden />
            {t("timeline.withheld")}
          </span>
        ) : (
          <span className="tl-title">{entry.title}</span>
        )}
        {/* Never for a withheld entry: the title above already says the
            content is for participants only, and a row must not show the
            words it just refused. The server withholds bodies upstream; this
            is the row keeping its own promise whatever it is handed.
            An email WITH its summary drew EmailEntry above and never reaches
            here; one without still needs the splitter, because a reader whose
            server has not caught up should not lose the fold. */}
        {entry.body && !entry.withheld && (
          <TimelineText text={entry.body} entry={entry} />
        )}
        {entry.detail}
        <span className="tl-meta">
          <ProvenanceTag provenance={entry.provenance} />
        </span>
        {entry.actions && <span className="tl-actions">{entry.actions}</span>}
      </div>
    </li>
  );
}
