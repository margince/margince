// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { X } from "lucide-react";
import type { ReactNode } from "react";
import { useCallback, useRef, useState } from "react";

import { Button } from "../design-system/atoms";
import { useFocusHandoff } from "../design-system/focushandoff";
import { Panel, PanelBody } from "../design-system/panel";
import { TagPill } from "../design-system/tagpill";
import { undoAction, useToast } from "../design-system/toast";
import { useTooltip } from "../design-system/tooltip";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { problemMessageOf } from "./common";
import { AddTagPicker } from "./tagpicker";
import type { RecordTag, TaggableType } from "./tags.queries";
import { useRecordTags, useRemoveTag, useRestoreTag } from "./tags.queries";
import "./tagspanel.css";

/**
 * The tags on one record — the same component on a contact, a company and a
 * deal, because a tag reads the same on all three and three panels would drift.
 *
 * Four visible, then "+N more". A record with twenty tags would otherwise push
 * everything below it off the screen, and the rail is where a reader looks for
 * the things they can act on rather than for a full inventory.
 *
 * The panel carries its own add verb rather than leaving it to each host, so a
 * record type cannot ship with tags a reader can see and no way to add one.
 */
const VISIBLE_TAGS = 4;

export function TagsPanel({
  entityType,
  entityID,
  canEdit,
  bare = false,
}: Readonly<{
  entityType: TaggableType;
  entityID: string;
  /** Drawn as the body of a section something else names, rather than as a
   * panel of its own: the details column's one pane carries the tags as one
   * of its named slices. */
  bare?: boolean;
  /** Whether this reader may change the record. Applying a tag writes to the
   * RECORD, so a reader who may only look at it sees the words and no verbs. */
  canEdit: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [expanded, setExpanded] = useState(false);
  // Where focus lands once a pill is gone: the Add tag row, which stays
  // mounted whenever a tag can be removed, including after the last one goes
  // and the row of tags unmounts with it.
  const actions = useRef<HTMLDivElement>(null);
  const focusLanding = useCallback(() => actions.current, []);
  const read = useRecordTags(entityType, entityID);

  // The frame stands while the read is in flight. This panel sits in the record
  // rail beside cards that keep their own frames as the composite read arrives,
  // and a card that appears only once its request lands makes the column reflow
  // under the reader's cursor. A failed read draws nothing: there is no honest
  // thing to say about a record's tags when the answer never came, and an empty
  // strip would claim it carries none.
  if (read.isError) {
    return null;
  }
  if (read.isPending) {
    return (
      <TagsFrame title={t("tags.panelTitle")} bare={bare}>
        <p className="tagspanel-note">{t("tags.loading")}</p>
      </TagsFrame>
    );
  }

  // Withheld is not empty. A caller who may read the record but not the
  // vocabulary is told the words are hidden — saying "no tags" would state a
  // fact about the record that nobody established.
  if (read.data.withheld) {
    return (
      <TagsFrame title={t("tags.panelTitle")} bare={bare}>
        <p className="tagspanel-note">{t("tags.withheld")}</p>
      </TagsFrame>
    );
  }

  // Default the list rather than trusting it. The contract makes `data`
  // required, so an answer without one is a server or a stub that disagrees
  // with the contract — and reading `.slice` off undefined takes the whole
  // RECORD PAGE down, not just this panel. A tag strip is not worth that.
  const tags = read.data.data ?? [];
  const visible = expanded ? tags : tags.slice(0, VISIBLE_TAGS);
  const hidden = tags.length - visible.length;

  return (
    <TagsFrame title={t("tags.panelTitle")} bare={bare}>
      {/* In a card of its own the empty panel teaches what tags are for. As
          a row of the Details card the label already says "Tags" and the
          dashed pill under it is the whole invitation, so the lesson would
          be two sentences beside every untagged record. */}
      {tags.length === 0 ? (
        !bare && (
          <div className="tagspanel-empty">
            <p className="tagspanel-empty-title">{t("tags.emptyTitle")}</p>
            <p className="tagspanel-note">{t("tags.emptyBody")}</p>
          </div>
        )
      ) : (
        <div className="tagspanel-set">
          {visible.map((tag) => (
            <TagOnRecord
              key={tag.tag_id}
              tag={tag}
              entityType={entityType}
              entityID={entityID}
              canEdit={canEdit}
              focusLanding={focusLanding}
            />
          ))}
          {hidden > 0 && (
            <Button variant="ghost" onClick={() => setExpanded(true)}>
              {t("tags.more", { count: formatNumber(hidden, locale) })}
            </Button>
          )}
          {expanded && tags.length > VISIBLE_TAGS && (
            <Button variant="ghost" onClick={() => setExpanded(false)}>
              {t("tags.showLess")}
            </Button>
          )}
        </div>
      )}
      {canEdit && (
        <div className="tagspanel-actions" ref={actions} tabIndex={-1}>
          <AddTagPicker
            entityType={entityType}
            entityID={entityID}
            current={tags}
          />
        </div>
      )}
    </TagsFrame>
  );
}

/**
 * The frame around the tags: a rail card, the same as every card beside it.
 *
 * The contact rail once drew these as a bare headed section instead. It sat
 * between two cards — the correspondence control above, recent activity below —
 * so the one section in the column read as unstyled rather than as a deliberate
 * second shape.
 */
function TagsFrame({
  title,
  bare,
  children,
}: Readonly<{
  title: string;
  bare: boolean;
  children: ReactNode;
}>) {
  // Bare is the set alone, for a host that already frames it: a row of the
  // record's Details card, where the card's own padding and the row's label
  // are the frame.
  if (bare) {
    return <>{children}</>;
  }
  return (
    <Panel title={title}>
      <PanelBody>{children}</PanelBody>
    </Panel>
  );
}

/**
 * One tag on one record: the word, and the cross that takes it off.
 *
 * The two are different things and the split pill says so. Clicking the word
 * goes to the tag — everything else carrying it. The cross acts on this record
 * alone, which is the distinction a reader has to be able to make before they
 * remove something.
 */
function TagOnRecord({
  tag,
  entityType,
  entityID,
  canEdit,
  focusLanding,
}: Readonly<{
  tag: RecordTag;
  entityType: TaggableType;
  entityID: string;
  canEdit: boolean;
  focusLanding: () => HTMLElement | null;
}>) {
  const t = useT();
  const combo = useRef<HTMLSpanElement | null>(null);
  const remove = useTagRemoval(entityType, entityID, tag.name);
  useFocusHandoff(combo, focusLanding);
  return (
    <span className="tagspanel-combo" ref={combo}>
      <TagLink tag={tag} />
      {/* A cross the pill grows on hover and focus, not a menu of one item.
          It runs at once; the toast's Undo restores the tagging as assigned. */}
      {canEdit && (
        <button
          type="button"
          className="tagspanel-remove"
          aria-label={t("tags.removeTag", { name: tag.name })}
          aria-disabled={remove.isPending || undefined}
          onClick={() => {
            if (!remove.isPending) {
              remove.mutate(tag.tag_id);
            }
          }}
        >
          <X aria-hidden />
        </button>
      )}
    </span>
  );
}

/** The word as a link to the tag, with who applied it here and when on hover and focus. */
function TagLink({ tag }: Readonly<{ tag: RecordTag }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  // Intl throws on an unparseable date, and a server out of step may send one.
  const stamped = !Number.isNaN(Date.parse(tag.assigned_at));
  const who = tag.assigned_by?.display_name;
  const when = stamped ? formatDate(tag.assigned_at, locale, zone) : "";
  // An assignment older than the product's record of WHO credits nobody,
  // rather than putting the choice on somebody.
  const added = who
    ? stamped
      ? t("tags.addedBy", { who, when })
      : t("tags.addedByUndated", { who })
    : stamped
      ? t("tags.addedOn", { when })
      : "";
  const provenance = useTooltip<HTMLAnchorElement>(added);
  return (
    <a
      ref={provenance.ref}
      className="tagspanel-open"
      href={`#/tags/${tag.tag_id}`}
      {...(added ? provenance.trigger : undefined)}
    >
      <TagPill name={tag.name} tone={tag.color} archived={tag.archived} />
      {provenance.tip}
    </a>
  );
}

// A refused removal or Undo has no dialog to stand in, so it stays as a toast.
function useTagRemoval(
  entityType: TaggableType,
  entityID: string,
  name: string,
) {
  const t = useT();
  const toast = useToast();
  const sayRefused = (error: Error) =>
    toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true });
  const restore = useRestoreTag(entityType, entityID, {
    onError: sayRefused,
    onSuccess: () => toast.show(t("tags.restored", { name })),
  });
  return useRemoveTag(entityType, entityID, {
    onError: sayRefused,
    onSuccess: (removal) =>
      toast.show(
        t("tags.removed", { name }),
        removal
          ? {
              action: undoAction(t("common.undo"), () =>
                restore.mutate(removal),
              ),
            }
          : undefined,
      ),
  });
}
