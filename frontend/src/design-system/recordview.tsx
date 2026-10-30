// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import { useT } from "../i18n";
import { ProblemError } from "../lib/problem";
import { AppLink } from "./applink";
import { Avatar } from "./atoms";
import { Heading } from "./heading";
import { InlineText } from "./inlinetext";
import { RecordStory } from "./recordstory";
import type { TimelineEntry, TimelineGroup } from "./timelineentry";
import "./recordview.css";

/** The way back to where the reader opened a record from, drawn over its head. */
export function RecordBack({
  href,
  label,
}: Readonly<{ href: string; label: string }>) {
  return (
    <div className="record-back">
      <AppLink className="link-button" href={href}>
        <ArrowLeft aria-hidden="true" />
        {label}
      </AppLink>
    </div>
  );
}

/** Renames the record where its name is read, in place like any details value. */
export type RecordRename = Readonly<{
  // The field's own name ("Full name"), which the edit verb reads out.
  label: string;
  // The stored name. It may be blank where `name` draws a stand-in.
  value: string;
  canEdit: boolean;
  readOnlyReason?: string;
  onSave: (next: string) => Promise<void>;
  onEditingChange?: (editing: boolean) => void;
  onDirtyChange?: (dirty: boolean) => void;
}>;

export type RecordViewProps = Readonly<{
  // A `RecordBack` over the head, or a component that may draw one. Absent
  // or null when the reader arrived any way that has no place to return to.
  back?: ReactNode;
  name: string;
  // Absent keeps the name as plain text.
  rename?: RecordRename;
  // The record's own image for the mark, a company's resolved logo. Null or
  // absent draws the deterministic monogram, which is the floor for every
  // record type that has no image at all.
  avatarSrc?: string | null;
  // One line under the name that is the record's standing (the company's
  // editable lifecycle badge), where a reader looks for it first.
  nameBadge?: ReactNode;
  // One line of joined facts, or a node for a record that needs structure
  // under its name.
  subtitle?: ReactNode;
  // A closing strip in the head (the contact's facts).
  badges?: ReactNode;
  // The record's state as pills: warmth, last touch, owner.
  pulse?: ReactNode;
  // The newest unanswered thing they sent, as one line under the pills, on
  // the tabs whose feed does not lead with it.
  latest?: ReactNode;
  // The record's verbs, in the head under the pills.
  actions?: ReactNode;
  // The record's own id, which keys its mark. Every list row and card drawing
  // this record passes the same key, so the head wears the same mesh.
  identity: string;
  // Full-width content over the feed that describes the whole record: its
  // readings, its stepper, the sentence that refuses an edit.
  band?: ReactNode;
  // The details under the head, in the left card. Absent draws the head alone.
  aside?: ReactNode;
  // The bar that chooses which part of the record the feed shows.
  tabs?: ReactNode;
  // The entries, or undefined when this view has NO timeline at all. The
  // distinction is the same one every card on a record page keeps: absent is
  // not empty. `[]` renders the section with its honest "nothing logged yet";
  // undefined omits the section, for a view whose body is not a history.
  timeline?: TimelineEntry[];
  // When set, the timeline renders CONVERSATIONS rather than messages. The
  // flat list stays the default: a contact's timeline is a handful of rows and
  // grouping it would collapse events that were never one.
  timelineGroups?: readonly TimelineGroup[];
  onOpenThread?: (threadKey: string) => void;
  // Controls above the timeline list (filters), and below it (load more).
  timelineHeader?: ReactNode;
  timelineFooter?: ReactNode;
  // Replaces the timeline list, e.g. a "not available" note, so the section
  // does not draw an empty list that reads as "no activity".
  timelineNotice?: ReactNode;
  // The id the timeline section carries, so a reading's door can scroll to the
  // record's story (lib/reveal) rather than route.
  timelineAnchorId?: string;
  zone: string;
  children?: ReactNode;
}>;

/**
 * The record as two cards. On the left, who the record is (its mark, name,
 * lines, pills and verbs) over its details. On the right, the tabs over the
 * open tab's feed. The DOM runs in the order the cards are drawn, so the
 * page's one `h1` is read before the feed.
 *
 * The details stay beside the feed while the page scrolls, and fold above it
 * once the row has no room for both cards.
 */
export function RecordView({
  back,
  aside,
  tabs,
  band,
  children,
  zone,
  timeline,
  timelineGroups,
  onOpenThread,
  timelineHeader,
  timelineFooter,
  timelineNotice,
  timelineAnchorId,
  ...head
}: RecordViewProps) {
  const t = useT();
  return (
    <div className="record-split">
      {back}
      <div className="record-split-row">
        <div className="record-split-details">
          <RecordHead {...head} />
          {aside && (
            <aside className="record-aside" aria-label={t("record.context")}>
              {aside}
            </aside>
          )}
        </div>
        <div className="record-split-sheet">
          {tabs && (
            <div className="record-split-tabs" data-testid="record-tabs">
              {tabs}
            </div>
          )}
          <div className="record-split-feed arrive-stack">
            {band && <div className="record-band">{band}</div>}
            {children}
            {timeline && (
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
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function RecordHead({
  name,
  rename,
  avatarSrc,
  nameBadge,
  subtitle,
  pulse,
  latest,
  badges,
  actions,
  identity,
}: Pick<
  RecordViewProps,
  | "name"
  | "rename"
  | "avatarSrc"
  | "nameBadge"
  | "subtitle"
  | "pulse"
  | "latest"
  | "badges"
  | "actions"
  | "identity"
>) {
  return (
    <header className="record-split-head">
      <div className="record-split-identity">
        <Avatar name={name} identity={identity} src={avatarSrc} size="md" />
        {rename?.canEdit ? (
          <RecordName name={name} rename={rename} />
        ) : (
          <RecordTitle name={name}>{name}</RecordTitle>
        )}
      </div>
      {nameBadge && <div className="record-split-lines">{nameBadge}</div>}
      {subtitle && <div className="record-split-lines">{subtitle}</div>}
      {pulse && <div className="record-split-pills">{pulse}</div>}
      {latest}
      {actions && <div className="record-split-verbs">{actions}</div>}
      {badges}
    </header>
  );
}

function RecordTitle({
  name,
  children,
}: Readonly<{ name: string; children: ReactNode }>) {
  return (
    <Heading
      size="large"
      as="h1"
      className="record-split-name"
      title={name}
      aria-label={name}
    >
      {children}
    </Heading>
  );
}

// The h1 holds the name's edit button at rest, and is named by the record, not
// by the button's verb. While open, the field stands in the heading's place.
function RecordName({
  name,
  rename,
}: Readonly<{ name: string; rename: RecordRename }>) {
  const t = useT();
  return (
    <div className="record-split-rename">
      <InlineText
        label={rename.label}
        value={rename.value}
        placeholder={name}
        canEdit
        frame={(resting) => <RecordTitle name={name}>{resting}</RecordTitle>}
        onEditingChange={rename.onEditingChange}
        onDirtyChange={rename.onDirtyChange}
        onSave={async (next) => {
          if (!next.trim()) {
            throw new ProblemError({ detail: t("record.fieldRequired") });
          }
          await rename.onSave(next);
        }}
      />
    </div>
  );
}
