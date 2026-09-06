// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { Badge } from "./atoms";
import "./tagpill.css";

/**
 * A tag, drawn as its word with a tone dot.
 *
 * Built ON Badge rather than widening it. Badge's tones are semantic — they
 * say what a thing IS (a status, a warning), and a reader learns to read them
 * that way. A tag's colour says nothing of the kind: an admin picks it so one
 * word is tellable from another at a glance, and hues carrying no meaning
 * inside a vocabulary of semantic ones would teach the reader the wrong
 * lesson about both.
 *
 * The dot is what carries the colour, not the pill. A filled pill per tag
 * turns a strip of tags into a stripe of blocks, and the words stop being
 * the thing you read.
 */

/**
 * The palette an admin picks from, in the order the picker offers it. Mirrors
 * the server's tag_color_check and the contract enum; every screen that offers
 * a colour reads THIS array rather than restating the list, so a tone added
 * here reaches the picker and the dot together or not at all.
 */
export const TAG_TONES = [
  "teal",
  "amber",
  "rose",
  "slate",
  "sky",
  "violet",
  "lime",
  "orange",
] as const;

export type TagTone = (typeof TAG_TONES)[number];

/**
 * isTagTone narrows a colour off the wire.
 *
 * The server constrains the column and the contract publishes the enum, so a
 * value outside the palette means the two have drifted. The pill then draws no
 * dot rather than an unstyled one — a tag with no colour is a tag, and a
 * broken swatch beside its name is a defect a reader has to interpret.
 */
export function isTagTone(value: string | null | undefined): value is TagTone {
  return TAG_TONES.some((tone) => tone === value);
}

export function TagPill({
  name,
  tone,
  archived,
}: Readonly<{
  name: string;
  tone?: string | null;
  /** An archived tag stays ON the record it was applied to. It draws muted
   * and says so, because a reader seeing it in a picker would look for a word
   * that is no longer offered. */
  archived?: boolean;
}>) {
  const t = useT();
  return (
    <Badge quiet={archived}>
      {!archived && isTagTone(tone) && (
        <span className={`tagpill-dot tagpill-dot-${tone}`} aria-hidden />
      )}
      {name}
      {archived ? ` · ${t("tags.archived")}` : ""}
    </Badge>
  );
}
