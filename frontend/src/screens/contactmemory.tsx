import { ChevronRight } from "lucide-react";
import { useState } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { activityTimeline } from "../design-system/activitytimeline";
import { Avatar, Badge, Button } from "../design-system/atoms";
import { EmailEntry } from "../design-system/emailentry";
import { FilterPills } from "../design-system/filterpills";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import {
  formatDayMonth,
  formatNumber,
  formatTimeOfDay,
} from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { ChannelReplyAction } from "./compose";
import { contactTabRoute } from "./contacttab";
import { interactionIcon, useInteractionLabel } from "./interactionchrome";
import { groupChronology } from "./timelinegroups";

// Conversation memory (concept §5.10, ADR-0097 D3).
//
// Threads and meetings as ENTITIES, condensed — what the conversation was
// about, not the transport events it was made of. The Activity tab remains the
// complete raw ledger beside it: a summary never replaces the original, and a
// withheld activity never leaks through one.
//
// It also carries the REPLY, and that is a deliberate exception to "summary,
// not ledger". A captured channel message links to a contact, and this card is
// the only routed surface a contact's channel conversations appear on — the
// timelines that mount the full action cluster are the deal, company and
// contact-list ones, and the last of those is not routed in this build. Without
// the reply here, a transport the installation can demonstrably send on has no
// button anywhere: the whole path exists, is governed, and is unreachable by
// the human it was built for.
//
// Only the reply, though. Relink is a raw-ledger act and stays on the ledger.

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];
type EmailSummary = components["schemas"]["EmailSummary"];

const FILTERS = ["all", "email", "meetings", "calls", "notes"] as const;
type Filter = (typeof FILTERS)[number];

// How many conversations the card draws before the footer takes over.
//
// The card is a GLANCE — what was said lately, read beside the card that says
// what is owed — and the two are a pair because they are read together. An
// uncut list breaks that: a contact with fifty captured exchanges makes this
// column longer than the page, and the other half of the reading leaves the
// screen. Three is what the rail's own recent-activity glance shows, so the
// two condensed lists on one record agree about how long a glance is.
const GLANCE = 3;

export function ContactMemory({
  view,
  onOpenEmail,
  hideEmpty = false,
}: Readonly<{
  view: Contact360;
  hideEmpty?: boolean;
  /**
   * Opens the page's email drawer. Optional because the page owns the drawer,
   * not this card: a host that mounts none passes none, and the rows stay
   * readable rather than offering a control that answers nothing.
   */
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [filter, setFilter] = useState<Filter>("all");
  const interactionLabel = useInteractionLabel();
  const entries = view.conversation_memory ?? [];
  // Until the thread projection is filled, the timeline the page already read
  // IS the memory: same rows, condensed the same way. It reads from what the
  // 360 assembled rather than fetching, so it cannot show a record the page
  // beside it is withholding.
  const rows =
    entries.length > 0
      ? entries.map((entry) =>
          fromEntry(
            entry,
            t,
            interactionLabel,
            locale,
            recordZone,
            view.contact.full_name,
          ),
        )
      : foldActivities(view, t, interactionLabel, locale, recordZone);
  // Filter first, then cut: three of what the reader asked for, not whichever
  // of the newest three survive the cut they chose. A filter that narrowed to
  // nothing already drawn would read as a channel with no history on it.
  const matching = rows.filter((row) => matches(row, filter));
  const shown = matching.slice(0, GLANCE);
  if (hideEmpty && rows.length === 0) return null;

  return (
    <Panel
      title={t("contact.memory.title")}
      // The way out of the glance, in the band that belongs to the whole card
      // rather than to any row. It lands on the Timeline tab — this record's
      // chronology whole, every exchange with the changes to the record beside
      // them — so it stands even when the glance happens to be drawing
      // everything the card folded, and a cut that came up empty needs it
      // most. The rail's own glance leads to the same tab: one record, one
      // ledger a reader is sent to.
      footer={
        <Button
          variant="link"
          onClick={() => navigate(contactTabRoute(view.contact.id, "timeline"))}
        >
          {t("contact.memory.showAll")} <ChevronRight aria-hidden="true" />
        </Button>
      }
    >
      {/* The cut sits UNDER the head, not in it. Five options do not fit beside
          the title on the one band a panel head is, and a strip that wrapped to
          a second row gave this card a head taller than every other card in the
          stack. Under it, the row reads as what it is: what narrows the list
          below it. FilterPills rather than a SegmentedControl, because these
          are cuts THROUGH a list and not a setting: the same row the History
          tab draws over the same conversations, so one record teaches one
          control. */}
      <PanelBody>
        <FilterPills
          pills={FILTERS.map((value) => ({
            value,
            label: t(`contact.memory.${value}`),
          }))}
          value={filter}
          onChange={setFilter}
          label={t("contact.memory.title")}
        />
      </PanelBody>
      {matching.length === 0 && (
        <PanelBody>
          <p className="pe-prose t-body">{t("contact.memory.empty")}</p>
        </PanelBody>
      )}
      {shown.map((row) => (
        <PanelRow className="meta-row" key={row.key}>
          {/* Who the row is with, as a face rather than a fact in the meta
              line: the counterparty on an email row, the contact themself on
              every other kind, so the column reads the same way a reader
              scans any list of contacts on this product. */}
          <Avatar name={row.who} size="sm" />
          {/* The meta line, as words: who the row is with, the transport the
              message came by (the directory's name for it with the kind's
              glyph, the one rule every source on this page follows, so a chat
              thread never reads as mail), how much of the conversation is
              folded behind the row, and its standing. A pill per fact made
              three rows carry nine pills. */}
          <span className="meta-row-line t-caption">
            <strong className="meta-row-who">{row.who}</strong>
            <span className="pe-source">
              {interactionIcon(row.kind)}
              {row.channelLabel}
            </span>
            {row.messageCount != null && row.messageCount > 1 && (
              <span>
                {t("co.spine.exchangeCount", {
                  count: formatNumber(row.messageCount, locale),
                })}
              </span>
            )}
            {row.status && (
              <span className="pe-memory-standing">
                <Badge tone={row.tone}>{row.statusLabel}</Badge>
              </span>
            )}
          </span>
          {/* The row's place in time, at the far end of the meta line rather
              than in its own gutter now that the gutter carries a face: the
              day and the hour, read together the way the History tab's
              chronology reads them. */}
          <span className="meta-row-time t-caption">
            {row.date} {row.time}
          </span>
          {/* Reply, on the same terms the 360 timelines offer it: available on
              any row, and WITHHELD on a channel row whose contact cannot be
              reached on the transport that carried it. Mail behaves exactly as
              it does there, the composer picks send-message over send-email
              from the row's kind, and nothing else about the interaction
              differs. A row with no anchor renders nothing. */}
          <span className="meta-row-action">
            {row.activityId && row.kind && (
              <ChannelReplyAction
                activityId={row.activityId}
                kind={row.kind}
                channelProvider={row.channelProvider ?? undefined}
                entityType="contact"
                entityId={view.contact.id}
                contactId={view.contact.id}
              />
            )}
          </span>
          {/* A retained email is the canonical row, whatever surface it is on.
              The card keeps its own avatar, meta line and time, those place
              the message in this card's reading, and hands the message
              itself to the one component that draws one. No timestamp on the
              entry: the meta line already says when, and the same figure
              twice on one row read as two events. */}
          {row.emailSummary ? (
            <EmailEntry
              summary={row.emailSummary}
              onOpen={openerFor(row, onOpenEmail)}
              // `openerFor` withholds an opener on two honest grounds: a page
              // that mounts no drawer, and a row the projection gave no
              // activity id. The second is the narrower claim, so it is the
              // one stated.
              whyNotOpenable="noDetail"
            />
          ) : (
            <span className="meta-row-entry">
              <span className="pe-memory-title">{row.title}</span>
              <span className="pe-memory-summary">{row.summary}</span>
            </span>
          )}
        </PanelRow>
      ))}
    </Panel>
  );
}

type Row = {
  messageCount?: number;
  key: string;
  // Who the row is drawn beside: the other party on a retained email, and the
  // contact themself on every other kind, since a note or a call has no
  // second name of its own to show.
  who: string;
  date: string;
  time: string;
  // What a reply anchors on, and the transport it would leave by. Null when the
  // row is not a channel message, or when the entry names no activity — a
  // thread projection is not obliged to carry one, and a reply anchored on
  // nothing is a button that can only fail.
  activityId: string | null;
  kind: Activity["kind"] | null;
  channelProvider: string | null;
  channel: string;
  channelLabel: string;
  title: string;
  summary: string;
  // The server's own row model for a retained email, when this row is one.
  // Present it and the card draws EmailEntry instead of its own two lines, so
  // a message reads here exactly as it does on the timeline beside it.
  //
  // Null on every other kind, and null for a thread-projection entry too: a
  // conversation summary is a fold of several messages and has no single
  // message's access state to show.
  emailSummary: EmailSummary | null;
  // `status` stays the STORED key and `statusLabel` is what the reader sees.
  // Folding them into one field would make the badge's tone depend on the
  // active locale, since tone is chosen by the same word.
  status: string | null;
  statusLabel: string;
  tone: "success" | "warning" | "accent" | undefined;
};

// The row's opener, or nothing.
//
// Both halves are required and neither is assumed: a page that mounts no
// drawer passes no opener, and a row the projection gave no activity id has
// nothing to open. EmailEntry draws an un-openable row as plain text, which is
// the honest reading — a row that looks openable and is not teaches a reader
// the product does not work.
function openerFor(
  row: Row,
  onOpenEmail: ((activityId: string) => void) | undefined,
): (() => void) | undefined {
  const { activityId } = row;
  if (!onOpenEmail || !activityId) {
    return undefined;
  }
  return () => onOpenEmail(activityId);
}

// The filter key for the channel column. Since ADR-0107/A158 a message names
// its transport separately, so the kind alone renders every channel row as the
// bare word "message" — a Telegram thread and a Dispact thread become
// indistinguishable. Folding the provider in keeps them apart, and falls back
// to the kind when a row has no transport (mail, calls, meetings, notes).
//
// It keys the segmented filter and NOTHING a reader sees: the name comes from
// the transport directory and the icon from the kind, so a provider this build
// has never heard of still reads, and no row is drawn as something it is not.
function channelKeyOf(
  kind: string,
  provider: string | null | undefined,
): string {
  return provider ?? kind;
}

// interactionLabel is useInteractionLabel's resolver, threaded in rather than
// called here: a Row is built outside the component, and a hook may not be.
// recordZone rides in the same way, for the same reason — useRecordZone is a
// hook, so the zone is read once in ContactMemory and passed down.
type InteractionLabel = ReturnType<typeof useInteractionLabel>;

// The row's counterparty, or the contact when the message names none of its
// own. A retained email carries who it was with; every other kind, and an
// email the server could not resolve a name for, reads as a conversation
// with the record the card is on.
function whoFor(
  emailSummary: EmailSummary | null,
  contactName: string,
): string {
  return emailSummary?.counterparty ?? contactName;
}

function fromEntry(
  entry: NonNullable<Contact360["conversation_memory"]>[number],
  t: ReturnType<typeof useT>,
  interactionLabel: InteractionLabel,
  locale: Locale,
  recordZone: string,
  contactName: string,
): Row {
  const status = entry.status ?? null;
  return {
    key: entry.key,
    // A thread-projection entry carries no email summary of its own, so the
    // counterparty it reads is always the contact this card is on.
    who: whoFor(null, contactName),
    date: formatDayMonth(entry.occurred_at, locale, recordZone),
    time: formatTimeOfDay(entry.occurred_at, locale, recordZone),
    // first_activity_id is what "expand to original" opens, and it is the right
    // anchor for a reply too: the send resolves the conversation from the
    // anchor's own links and thread key, so the first message of a thread names
    // the same conversation as its last.
    activityId: entry.first_activity_id ?? null,
    // The row's own kind, whatever it is. A reply is offered on a mail row
    // exactly as it is on a channel one — the same rule the 360 timelines
    // apply — because a composer gated to channel rows would leave a workspace
    // whose only rows are mail unable to answer anything from here.
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
function foldActivities(
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
    const row = byId.get(group.entries[0].id);
    if (!row) return [];
    const status = statusOf(row, rows);
    const withheldRow = row.content_state === "withheld";
    return {
      key: row.id,
      messageCount: group.entries.length,
      who: whoFor(row.email_summary ?? null, view.contact.full_name),
      date: formatDayMonth(row.occurred_at, locale, recordZone),
      time: formatTimeOfDay(row.occurred_at, locale, recordZone),
      activityId: row.id,
      kind: row.kind,
      channelProvider: row.channel_provider ?? null,
      channel: channelKeyOf(row.kind, row.channel_provider),
      channelLabel: interactionLabel(row.kind, row.channel_provider),
      // Withheld is read from the row's own state rather than trusted to
      // have been stripped. The server does strip it — both readers of the
      // activity table null the subject and the body together — but this
      // card would otherwise fall through to row.subject, and a fallback
      // chain that only holds because of what the server sends is one
      // response away from printing a subject it should not.
      title: withheldRow
        ? t("email.withheldSubject")
        : (row.email_summary?.subject ??
          row.subject ??
          interactionLabel(row.kind, row.channel_provider)),
      // The server's own preview for an email, composed with the same
      // splitter the drawer folds with — so this card and the message it
      // opens cannot disagree about where the sender's words end.
      summary: withheldRow
        ? ""
        : row.email_summary
          ? (row.email_summary.preview ?? "")
          : (row.body ?? ""),
      // A retained email draws the canonical row. The title and summary
      // above stay filled for it: they are what the segmented filter reads,
      // and what the row falls back to if a server has not caught up.
      emailSummary: row.email_summary ?? null,
      status,
      statusLabel: statusLabel(status, t),
      tone: toneFor(status),
    };
  });
}

// A meeting that has not happened is not memory. It is on the strip and in the
// Today card; repeating it here as something remembered would be wrong.
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

// A status the server sends but this client has no word for is shown as it was
// stored rather than dropped: an unlabelled badge is still evidence, an absent
// one is a claim that the exchange has no status.
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

function matches(row: Row, filter: Filter): boolean {
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

// When a conversation happened is a fact about the RECORD, not about where the
// reader is sitting: two colleagues discussing the same thread have to name the
// same day. The exact time rides beside it because a reader deciding whether to
// follow up wants the hour and not "recently".
