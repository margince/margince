// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowLeft, Lock } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useT } from "../i18n";
import { Avatar } from "./atoms";
import { Heading } from "./heading";
import { PageZones } from "./pagezones";
import { createPortalSlot, SlotFill } from "./portalslot";
import {
  actionsPlacement,
  headMarkSize,
  zonesClassName,
  zonesShape,
} from "./recordview.layout";
import { GroupedTimelineList, TimelineList } from "./timeline";
import type { TimelineEntry, TimelineGroup } from "./timelineentry";
import { useTruncationTooltip } from "./tooltip";

// The identity block: who this record is, and the verbs and standing that
// belong beside the name rather than under it. Split from RecordView because
// the two answer different questions — this one what the record IS, the other
// how its columns are laid out — and reading either meant holding both.
function RecordHead({
  name,
  avatarSrc,
  nameBadge,
  subtitle,
  pulse,
  badges,
  controls,
  actions,
  actionsAt,
  standing,
  wide,
  compact = false,
  identity,
  markShape,
}: Readonly<{
  name: string;
  avatarSrc?: string | null;
  nameBadge?: ReactNode;
  subtitle?: ReactNode;
  pulse?: ReactNode;
  badges?: ReactNode;
  controls?: ReactNode;
  actions?: ReactNode;
  actionsAt: "none" | "inline" | "controls" | "below";
  // Where the record STANDS — a deal's stage ladder, a lead's. Its own row at
  // the foot of the head, under the facts and the verbs. It belongs to the
  // head rather than between the head and the tab strip, because the strip
  // sits one interval under the identity on every record in this product: a
  // block in that gap opens the choice of what to read a block lower here than
  // on the record beside it. Inside the head, the ladder is part of the
  // identity it describes and the interval below is unchanged.
  standing?: ReactNode;
  wide: boolean;
  // One rung under the record scale: the mark at `lg` and the name at the h1
  // rung, for a page whose head shares the fold with the work below it.
  compact?: boolean;
  identity: string;
  markShape?: "round" | "tile";
}>) {
  const nameTip = useTruncationTooltip<HTMLHeadingElement>(name);
  return (
    <header
      className={[
        "record-head",
        wide ? "record-head-wide" : "",
        compact ? "record-head-compact" : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {/* The wide header's chip is the page's own mark rather than a marker
          beside a name, so it takes the largest rung. This used to be a
          `.record-head-wide .avatar` override in composed.css, which meant the
          chip's size was decided by a class on its parent and the `size` prop
          said something that was not true. */}
      <Avatar
        name={name}
        identity={identity}
        src={avatarSrc}
        size={headMarkSize(wide, compact)}
        shape={markShape}
      />
      <div className="record-id">
        {/* The record page's name, and the one badge that belongs on ITS
            OWN line — a record's standing, read immediately after what it
            is named, not one fact among the others under it. A div, not a
            p, for the same reason as record-sub below: a caller passing
            structure there must not land inside a paragraph the browser
            silently un-nests. */}
        <div className="record-name-row">
          {/* The shell's page head yields to it on a record route — it
              prints the trail that leads here and nothing at heading
              level, so this stays the page's one h1. */}
          {/* A record's name is user data of unbounded length. It is drawn on
              one line and truncated rather than allowed to grow the header;
              the tooltip is what carries the whole of it, and appears only
              when there was more name than row. */}
          <Heading size="xxlarge" ref={nameTip.ref} {...nameTip.trigger}>
            {name}
            {nameTip.tip}
          </Heading>
          {nameBadge}
        </div>
        {/* A div, not a p: a caller passing structure — the company page's
            description line plus its chip row — would otherwise nest block
            elements inside a paragraph, which the browser silently un-nests,
            leaving the chips outside the header they belong to. */}
        {subtitle && <div className="record-sub">{subtitle}</div>}
        {pulse && <div className="record-pulse">{pulse}</div>}
      </div>
      {badges && <div className="record-badges">{badges}</div>}
      {/* The record's standing and its verbs, stacked at the top right. Only a
          caller that passes `controls` gets this column: every other record
          keeps the action row under the header, which is where its own layout
          puts it. */}
      {controls && (
        <div className="record-controls">
          {controls}
          {actionsAt === "controls" && (
            <div className="record-actions">{actions}</div>
          )}
        </div>
      )}
      {actionsAt === "inline" && (
        <div className="record-actions record-actions-inline">{actions}</div>
      )}
      {standing && <div className="record-standing">{standing}</div>}
    </header>
  );
}

/** Where a record says why its verbs are refused (`RecordRefusal`). */
const RecordAccessSlot = createPortalSlot();

/**
 * RecordRefusal is one sentence saying why a record's verbs are refused, kept
 * by `id` so each refused verb names it as its description. It stands in the
 * access note at the foot of the fields column, where a reader looks for what
 * they may do with the record. While that column is folded, it stands in the
 * head's verb row.
 */
export function RecordRefusal({
  id,
  children,
}: Readonly<{ id: string; children: ReactNode }>) {
  return (
    <SlotFill slot={RecordAccessSlot} className="record-refusal">
      <p className="t-caption" id={id}>
        {children}
      </p>
    </SlotFill>
  );
}

/** The way back to where the reader opened a record from, drawn over its head. */
export function RecordBack({
  href,
  label,
}: Readonly<{ href: string; label: string }>) {
  return (
    <div className="record-back">
      <a className="link-button" href={href}>
        <ArrowLeft aria-hidden="true" />
        {label}
      </a>
    </div>
  );
}

export function RecordView({
  back,
  name,
  avatarSrc,
  nameBadge,
  subtitle,
  badges,
  pulse,
  actions,
  controls,
  identity,
  markShape,
  actionsInline,
  scale = "record",
  standing,
  band,
  rail,
  railLabel,
  aside,
  asideOpen,
  asideToggle,
  asideLabel,
  timeline,
  timelineGroups,
  onOpenThread,
  timelineHeader,
  timelineFooter,
  timelineNotice,
  timelineAnchorId,
  tabs,
  zone,
  children,
}: Readonly<{
  // A `RecordBack` over the head, or a component that may draw one. Absent
  // or null when the reader arrived any way that has no place to return to.
  back?: ReactNode;
  name: string;
  // The record's own image for the header chip — a company's resolved logo.
  // Null or absent renders the deterministic monogram, which is the floor for
  // every record type that has no image at all.
  avatarSrc?: string | null;
  // The record's standing, read on the SAME line as its name rather than as
  // one more fact under it — the company page's editable lifecycle badge.
  // Absent on every record that has no such single, always-shown value.
  nameBadge?: ReactNode;
  // A string for the records whose subtitle IS one line of joined facts, or a
  // node for a record that needs structure under its name — the company page's
  // editable description plus its row of attribute chips.
  subtitle?: ReactNode;
  badges?: ReactNode;
  // A one-line "state of this record" strip under the name — warmth, last
  // touch, owner. Absent on records that have no such summary.
  pulse?: ReactNode;
  // The record's verbs, kept beside the identity rather than scattered
  // through the body.
  actions?: ReactNode;
  // The record's standing — the values a reader changes in place rather than
  // acts on: lifecycle, owner. Passing it moves the action row up beside them,
  // which is the company page's layout; a record that passes none keeps the
  // action row under the header.
  controls?: ReactNode;
  // The record's own id, which keys its mark: the same key every list row and
  // card drawing this record passes, so the header wears the same mesh.
  identity: string;
  // The mark's shape: a tile for a company or a deal, round for a contact.
  markShape?: "round" | "tile";
  // Puts `actions` on the SAME row as the identity block, right-aligned,
  // instead of the default full-width row underneath the header (or the
  // stacked column `controls` produces). An explicit opt-in: every other
  // record keeps its actions where its own layout already puts them.
  actionsInline?: boolean;
  // How large the identity is drawn. `record` is the masthead every record
  // page opens on; `compact` takes it one rung down (mark at `lg`, name at the
  // h1 rung) for a page whose head has to share the first fold with the
  // agent's ask under it.
  scale?: "record" | "compact";
  // The three-zone record page: rail is the left column (what this record
  // IS), children the middle (what is happening), aside the right (the
  // business around it). With neither rail nor aside the layout collapses
  // to the single column every existing caller already renders.
  // Full-width content between the tab strip and the columns: what describes
  // the WHOLE record — its readings, its stepper, the refusal of an edit.
  /**
   * Where the record STANDS, above the choice of what to read about it: a
   * deal's stage ladder. Over the tabs rather than under them, because the
   * ladder answers the question a reader arrives with, and a reader who has
   * already chosen a tab has passed it.
   */
  // Where the record stands, drawn at the foot of its head — see RecordHead.
  standing?: ReactNode;
  band?: ReactNode;
  rail?: ReactNode;
  // What the rail and the aside ARE, for a reader navigating by landmark: each
  // defaults to what its column usually holds, and one holding something else
  // names it. A page with a Profile TAB and a Profile RAIL is two regions of
  // one name, one of them wrong, and a dead end for anyone moving between them.
  railLabel?: string;
  aside?: ReactNode;
  // Whether the details pane STANDS OPEN: `PageZones`'s own prop, handed
  // through, so a pane a toggle shut folds its column instead of vanishing.
  asideOpen?: boolean;
  // The pane's own switch: in its head while open, and the strip that
  // brings it back while folded.
  asideToggle?: ReactNode;
  asideLabel?: string;
  // The entries, or undefined when this view has NO timeline at all. The
  // distinction is the same one every card on a record page keeps: absent is
  // not empty. `[]` renders the section with its honest "nothing logged yet";
  // undefined omits the section, for a view whose body is not a history.
  timeline?: TimelineEntry[];
  /**
   * When set, the timeline renders CONVERSATIONS rather than messages. The
   * flat list stays the default: a contact's timeline is a handful of rows and
   * grouping it would collapse events that were never one.
   */
  timelineGroups?: readonly TimelineGroup[];
  onOpenThread?: (threadKey: string) => void;
  // Controls above the timeline list (filters), and below it (load more).
  timelineHeader?: ReactNode;
  timelineFooter?: ReactNode;
  // When set, replaces the timeline list — e.g. a "not available"
  // note, since the mirror cannot serve entity-scoped activity reads. Keeps the
  // section honest instead of rendering an empty list that reads as "no activity".
  timelineNotice?: ReactNode;
  // The id the timeline section carries, so a reading's door can scroll to the
  // record's story (lib/reveal) rather than route. It belongs here because the
  // section is inside the work column this component draws. There a caller has
  // nothing to wrap an anchor of its own around.
  timelineAnchorId?: string;
  // The bar that chooses which part of the record is below it. It runs the
  // full width over the columns, because the details pane opens under it
  // (DESIGN.md §6): the switch at the row's end governs the column beside the
  // work, and a strip confined to the work column would end where the pane it
  // opens begins. The slot exists so the interval under it and the
  // one-row-that-scrolls behaviour are the record page's, spelled once — two
  // pages had already written the same wrapper under two names.
  tabs?: ReactNode;
  zone: string;
  children?: ReactNode;
}>) {
  const t = useT();
  const [accessNote, setAccessNote] = useState<HTMLElement | null>(null);
  // The grid follows which slots are actually filled, because a three-column
  // template with an empty column does not collapse: it reserves the space and
  // leaves the story narrower than the rail beside it.
  const shape = zonesShape(Boolean(rail), Boolean(aside));
  // Also when the verbs sit on the identity's own row: that record's block is
  // a name over a description, a chip row and a meta line, and centring the
  // mark against a stack that tall floats it to the middle of the chips
  // instead of beside the name it belongs to.
  //
  // These are the SAME two slots `actionsPlacement` reads, which is what makes
  // "a wide head never puts its verbs in the row below it" a fact about the
  // component rather than a convention its callers keep: a head is wide only
  // if it was handed `controls` or `actionsInline`, and either of those places
  // the verbs inside the header. The record-head-wide rules in composed.css
  // rest on that.
  const headerWide = Boolean(controls) || Boolean(actionsInline);
  const actionsAt = actionsPlacement(actions, actionsInline, controls);
  const head = (
    <RecordHead
      name={name}
      avatarSrc={avatarSrc}
      nameBadge={nameBadge}
      subtitle={subtitle}
      pulse={pulse}
      badges={badges}
      controls={controls}
      actions={actions}
      actionsAt={actionsAt}
      standing={standing}
      wide={headerWide}
      compact={scale === "compact"}
      identity={identity}
      markShape={markShape}
    />
  );
  // The strip sits directly under the identity on EVERY record, band or not: a
  // record with readings would otherwise open the choice of what to read a
  // block lower than the record beside it.
  const strip = tabs && (
    <div className="record-tabs" data-testid="record-tabs">
      {tabs}
    </div>
  );
  // What describes the WHOLE record frames the columns from between the strip
  // and them at full width, not from the work column beside the rail.
  const frame = band && <div className="record-band">{band}</div>;
  const story = timeline && (
    <RecordStory
      timeline={timeline}
      timelineGroups={timelineGroups}
      onOpenThread={onOpenThread}
      timelineHeader={timelineHeader}
      timelineFooter={timelineFooter}
      timelineNotice={timelineNotice}
      timelineAnchorId={timelineAnchorId}
      zone={zone}
    />
  );
  return (
    /* The record is one sheet, its head, tabs and the open tab's work, with
       its fields beside it on the page ground rather than on the sheet: the
       sheet is what the record says, the column what it is filed as. Its
       blocks arrive in order (head, strip, band, then the tab's work), and
       the sheet is an `.arrive-stack` rather than one plate fading in. */
    <RecordAccessSlot.Provider value={accessNote}>
      <div className="arrive-stack">
        {back}
        <PageZones
          shape={shape}
          className={zonesClassName(shape)}
          rail={rail}
          railLabel={railLabel ?? t("record.profile")}
          railClassName="record-rail"
          /* A tab's panel is a fresh element while the strip above it is not,
           so switching tabs fades the new panel in without touching the
           strip. That is the whole tab-panel transition. */
          mainClassName="record-sheet arrive-stack"
          main={
            <>
              {head}
              {actionsAt === "below" && (
                <div className="record-actions">{actions}</div>
              )}
              {strip}
              {frame}
              {children}
              {story}
            </>
          }
          aside={
            aside && (
              <>
                {asideToggle && (
                  <div className="record-aside-head">{asideToggle}</div>
                )}
                {aside}
                {/* Empty, and then not drawn, until a refusal fills it. */}
                <div className="record-access">
                  <Lock aria-hidden="true" />
                  <div className="record-access-lines" ref={setAccessNote} />
                </div>
              </>
            )
          }
          asideOpen={asideOpen}
          asideFold={asideToggle}
          asideLabel={asideLabel ?? t("record.context")}
          asideClassName="record-aside"
        />
      </div>
    </RecordAccessSlot.Provider>
  );
}

function RecordStory({
  timeline,
  timelineGroups,
  onOpenThread,
  timelineHeader,
  timelineFooter,
  timelineNotice,
  timelineAnchorId,
  zone,
}: Readonly<{
  timeline: TimelineEntry[];
  timelineGroups?: readonly TimelineGroup[];
  onOpenThread?: (threadKey: string) => void;
  timelineHeader?: ReactNode;
  timelineFooter?: ReactNode;
  timelineNotice?: ReactNode;
  timelineAnchorId?: string;
  zone: string;
}>) {
  const t = useT();
  return (
    <section
      id={timelineAnchorId}
      className="record-timeline"
      aria-label={t("record.timeline")}
    >
      <Heading size="large" className="t-sub">
        {t("record.timeline")}
      </Heading>
      {/* The dials above the list are one block with one rhythm: the cuts
          through the chronology, then the narrowing of whichever cut is open.
          Rendered as bare siblings they touched, and two rows of controls
          with no interval between them read as one control that has
          wrapped. */}
      {timelineHeader && (
        <div className="timeline-header">{timelineHeader}</div>
      )}
      {/* The chronology sits on a card of its own, like every other body on
          the page. Loose on the page ground it read as the page's own text
          rather than as one of the record's sections, and the rail down its
          left had nothing to run inside. The notice takes no card: a sentence
          about why there are no rows is not a list of them. */}
      {timelineNotice ?? (
        <div className="timeline-card">
          {timelineGroups ? (
            <GroupedTimelineList
              groups={timelineGroups}
              zone={zone}
              onOpenThread={onOpenThread}
            />
          ) : (
            <TimelineList entries={timeline} zone={zone} />
          )}
        </div>
      )}
      {timelineFooter}
    </section>
  );
}
