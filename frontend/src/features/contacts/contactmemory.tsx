import { ChevronRight } from "lucide-react";
import { useState } from "react";
import type { components } from "../../api/schema";
import { Avatar, Badge, Button } from "../../design-system/atoms";
import { EmailEntry } from "../../design-system/emailentry";
import { FilterPills } from "../../design-system/filterpills";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import {
  interactionIcon,
  useInteractionLabel,
} from "../../screens/interactionchrome";
import { ChannelReplyAction } from "../mail/compose";
import { openContactTab } from "../worklist/worklist.return";

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
import {
  foldActivities,
  fromEntry,
  matches,
  openerFor,
} from "./contactmemory.rows";

export type Contact360 = components["schemas"]["Contact360"];

export type Activity = components["schemas"]["Activity"];

export type EmailSummary = components["schemas"]["EmailSummary"];

const FILTERS = ["all", "email", "meetings", "calls", "notes"] as const;

export type Filter = (typeof FILTERS)[number];

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
  // When a conversation happened is a fact about the record, so it reads in the
  // record's zone. Two colleagues discussing one thread must name the same day.
  // The time rides beside it: a reader deciding whether to follow up wants the hour.
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
            view.contact,
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
          onClick={() => openContactTab(view.contact.id, "timeline")}
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
          <Avatar name={row.who.name} identity={row.who.key} size="sm" />
          {/* The meta line, as words: who the row is with, the transport the
              message came by (the directory's name for it with the kind's
              glyph, the one rule every source on this page follows, so a chat
              thread never reads as mail), how much of the conversation is
              folded behind the row, and its standing. A pill per fact made
              three rows carry nine pills. */}
          <span className="meta-row-line t-caption">
            <strong className="meta-row-who">{row.who.name}</strong>
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
