// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useT } from "../i18n";
import { Avatar } from "./atoms";
import {
  GroupedTimelineList,
  type TimelineEntry,
  type TimelineGroup,
  TimelineList,
} from "./composed";
import { PageZones, type PageZonesShape } from "./pagezones";
import { useTruncationTooltip } from "./tooltip";

// Where a record's verbs land. Three places can hold them — beside the
// standing column, on the identity's own row, or in a band under the header —
// and the choice is made once here rather than restated as a condition at
// each of the three, where a reader had to hold all three at once to know
// which one wins.
function actionsPlacement(
  actions: ReactNode,
  inline: boolean | undefined,
  controls: ReactNode,
): "none" | "inline" | "controls" | "below" {
  if (!actions) {
    return "none";
  }
  if (inline) {
    return "inline";
  }
  return controls ? "controls" : "below";
}

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
  wide,
  compact = false,
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
  wide: boolean;
  // One rung under the record scale: the mark at `lg` and the name at the h1
  // rung, for a page whose head shares the fold with the work below it.
  compact?: boolean;
  markShape: "contact" | "company";
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
          <h1 ref={nameTip.ref} {...nameTip.trigger}>
            {name}
            {nameTip.tip}
          </h1>
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
    </header>
  );
}

export function RecordView({
  name,
  avatarSrc,
  nameBadge,
  subtitle,
  badges,
  pulse,
  actions,
  controls,
  markShape = "contact",
  actionsInline,
  scale = "record",
  band,
  rail,
  railLabel,
  aside,
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
  // What KIND of record this is, which decides whether its mark is drawn round
  // like a face or as a rounded square like a logo. Defaults to `contact`,
  // which is what every record but a company is.
  markShape?: "contact" | "company";
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
  band?: ReactNode;
  rail?: ReactNode;
  // What the rail column IS, on the same rule as asideLabel below: it defaults
  // to the record's profile because that is what a rail usually holds, and a
  // page whose rail holds something else names it. A record page that also has
  // a Profile TAB is exactly that case — two regions called "Profile", one of
  // them wrong, is a dead end for anyone navigating by landmark.
  railLabel?: string;
  aside?: ReactNode;
  // What the aside column IS, for a reader navigating by landmark. Defaults to
  // the record's context; a page whose aside holds something else names it,
  // because two regions with one name is a dead end for anyone moving between
  // them.
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
  // When set, replaces the timeline list — e.g. an overlay-mode "not available"
  // note, since the mirror cannot serve entity-scoped activity reads. Keeps the
  // section honest instead of rendering an empty list that reads as "no activity".
  timelineNotice?: ReactNode;
  // The id the timeline SECTION carries, so a reading's door can scroll to the
  // record's story (app/reveal) rather than route. It belongs here because the
  // section is inside the work column this component draws, where a caller has
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
      wide={headerWide}
      compact={scale === "compact"}
      markShape={markShape}
    />
  );
  // The strip sits directly under the identity on EVERY record, band or not: a
  // record with readings would otherwise open the choice of what to read a
  // block lower than the record beside it.
  const strip = tabs && <div className="record-tabs">{tabs}</div>;
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
    /* The record's own blocks arrive in order: head, then actions, then the
       strip, then the band, then the zones, rather than the whole record
       fading in as one plate. It is an `.arrive-stack` and therefore not
       itself an arriving block (design-system/enter.css), which is what keeps
       the two fades from multiplying. */
    <div className="arrive-stack">
      {head}
      {actionsAt === "below" && <div className="record-actions">{actions}</div>}
      {strip}
      {frame}
      <PageZones
        shape={shape}
        className={zonesClassName(shape)}
        rail={rail}
        railLabel={railLabel ?? t("record.profile")}
        railClassName="record-rail"
        /* An `.arrive-stack`: the work column's blocks arrive one after the
           next, and, because a tab's panel is a fresh element while the strip
           above it is not, switching tabs fades the new panel in without
           touching the strip. That is the whole tab-panel transition; no
           wrapper, no state, and nothing to keep in step with the strip. */
        mainClassName="arrive-stack"
        main={
          <>
            {children}
            {story}
          </>
        }
        aside={aside}
        asideLabel={asideLabel ?? t("record.context")}
        asideClassName="record-aside"
      />
    </div>
  );
}

// The record's story, under whatever the open tab drew. It owns the break
// above it because the work column owns no interval: the deal's and the
// project's bodies met the chronology's heading at the border.
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
      <h2 className="t-sub">{t("record.timeline")}</h2>
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

// The mark's rung follows the head's: the masthead draws it at the record
// rung, the compact head one under, and the narrow head keeps the list's.
function headMarkSize(wide: boolean, compact: boolean): "md" | "lg" | "xl" {
  if (compact) {
    return "lg";
  }
  return wide ? "xl" : "md";
}

// Which columns this record actually has. The grid itself is `PageZones` — a
// record page is one page shape among others, and the ratios and the folds are
// not the record's to own.
function zonesShape(hasRail: boolean, hasAside: boolean): PageZonesShape {
  if (hasRail && hasAside) {
    return "both";
  }
  if (hasRail) {
    return "rail";
  }
  if (hasAside) {
    return "aside";
  }
  return "single";
}

// What the record adds to the grid container on top of the layout.
//
// `arrive-stack` on every shape, including the one with no columns: a record's
// blocks arrive individually (design-system/enter.css), and a container that is
// a stack does not itself arrive. Leaving one link of that chain unmarked is
// what makes a block fade in BEHIND a parent that is still fading in — two
// fades multiplied, which reads as the content being dim rather than as it
// arriving.
//
// `record-zones` carries only the phone bottom clearance for the sticky action
// bar (composed.css), which is why the single-column shape does not get it: a
// record with no columns never had it either.
function zonesClassName(shape: PageZonesShape): string {
  return shape === "single" ? "arrive-stack" : "record-zones arrive-stack";
}
