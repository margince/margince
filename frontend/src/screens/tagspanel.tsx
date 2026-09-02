// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { MoreHorizontal, Plus } from "lucide-react";
import { useState } from "react";

import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import { TagPill } from "../design-system/tagpill";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { RecordTag, TaggableType } from "./tags.queries";
import { useRecordTags, useRemoveTag } from "./tags.queries";
import "./tagspanel.css";

/**
 * The tags on one record — the same component on a person, a company and a
 * deal, because a tag reads the same on all three and three panels would drift.
 *
 * Four visible, then "+N more". A record with twenty tags would otherwise push
 * everything below it off the screen, and the rail is where a reader looks for
 * the things they can act on rather than for a full inventory.
 */
const VISIBLE_TAGS = 4;

export function TagsPanel({
  entityType,
  entityID,
  canEdit,
}: Readonly<{
  entityType: TaggableType;
  entityID: string;
  /** Whether this reader may change the record. Applying a tag writes to the
   * RECORD, so a reader who may only look at it sees the words and no verbs. */
  canEdit: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [expanded, setExpanded] = useState(false);
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
      <Panel title={t("tags.panelTitle")}>
        <PanelBody>
          <p className="tagspanel-note">{t("tags.loading")}</p>
        </PanelBody>
      </Panel>
    );
  }

  // Withheld is not empty. A caller who may read the record but not the
  // vocabulary is told the words are hidden — saying "no tags" would state a
  // fact about the record that nobody established.
  if (read.data.withheld) {
    return (
      <Panel title={t("tags.panelTitle")}>
        <PanelBody>
          <p className="tagspanel-note">{t("tags.withheld")}</p>
        </PanelBody>
      </Panel>
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
    <Panel
      title={t("tags.panelTitle")}
      sub={tags.length > 0 ? t("tags.panelSub") : undefined}
    >
      <PanelBody>
        {tags.length === 0 ? (
          <div className="tagspanel-empty">
            <p className="tagspanel-empty-title">{t("tags.emptyTitle")}</p>
            <p className="tagspanel-note">{t("tags.emptyBody")}</p>
          </div>
        ) : (
          <div className="tagspanel-set">
            {visible.map((tag) => (
              <TagOnRecord
                key={tag.tag_id}
                tag={tag}
                entityType={entityType}
                entityID={entityID}
                canEdit={canEdit}
              />
            ))}
            {hidden > 0 && (
              <Button small variant="ghost" onClick={() => setExpanded(true)}>
                {t("tags.more", { count: formatNumber(hidden, locale) })}
              </Button>
            )}
            {expanded && tags.length > VISIBLE_TAGS && (
              <Button small variant="ghost" onClick={() => setExpanded(false)}>
                {t("tags.showLess")}
              </Button>
            )}
          </div>
        )}
      </PanelBody>
    </Panel>
  );
}

/**
 * One tag on one record: the word, and a menu for the ASSIGNMENT.
 *
 * The two are different things and the split pill says so. Clicking the word
 * goes to the tag — everything else carrying it. The menu acts on this record
 * alone, which is the distinction a reader has to be able to make before they
 * remove something.
 */
function TagOnRecord({
  tag,
  entityType,
  entityID,
  canEdit,
}: Readonly<{
  tag: RecordTag;
  entityType: TaggableType;
  entityID: string;
  canEdit: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const remove = useRemoveTag(entityType, entityID);

  return (
    <span className="tagspanel-combo">
      <a className="tagspanel-open" href={`#/tags/${tag.tag_id}`}>
        <TagPill name={tag.name} tone={tag.color} archived={tag.archived} />
      </a>
      <Popover
        variant="ghost"
        className="tagspanel-more"
        label={
          <>
            <MoreHorizontal aria-hidden />
            <span className="sr-only">
              {t("tags.options", { name: tag.name })}
            </span>
          </>
        }
      >
        <div className="tagspanel-menu">
          <strong>{tag.name}</strong>
          <p className="t-caption">
            {tag.assigned_by?.display_name
              ? t("tags.addedBy", {
                  who: tag.assigned_by.display_name,
                  when: formatDate(tag.assigned_at, locale, zone),
                })
              : // No name where the row records none: an assignment written
                // before the product kept one has nobody to credit, and
                // inventing a name would put a choice on somebody.
                t("tags.addedOn", {
                  when: formatDate(tag.assigned_at, locale, zone),
                })}
          </p>
          <span className="t-caption">{t("tags.visibleWorkspaceWide")}</span>
          {canEdit && (
            <Button
              small
              variant="ghost"
              disabled={remove.isPending}
              onClick={() => remove.mutate(tag.tag_id)}
            >
              {t("tags.removeFromRecord")}
            </Button>
          )}
        </div>
      </Popover>
    </span>
  );
}

/** The add-tag verb, which the caller places where the record page wants it. */
export function AddTagButton({ onOpen }: Readonly<{ onOpen: () => void }>) {
  const t = useT();
  return (
    <Button small variant="ghost" onClick={onOpen}>
      <Plus aria-hidden /> {t("tags.add")}
    </Button>
  );
}
