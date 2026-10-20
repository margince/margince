// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarClock } from "lucide-react";
import { Avatar } from "./avatar";
import "./recordchip.css";

/** A contact or an account draws its avatar; a meeting draws a clock. */
export type RecordChipKind = "contact" | "company" | "meeting";

/**
 * One record named inside a sentence, as a pill the reader can press.
 *
 * A press shows what the sentence was read from, next to the sentence, so the
 * chip never navigates: the caller owns `pressed` and what it opens. A record
 * only named in a line, with a page to go to, is an `.entity-link` instead.
 */
export function RecordChip({
  kind,
  label,
  identity,
  pressed,
  onPress,
}: Readonly<{
  kind: RecordChipKind;
  label: string;
  /** The record's own id, so the avatar keys the same colour as elsewhere. */
  identity: string;
  pressed: boolean;
  onPress: () => void;
}>) {
  return (
    <button
      type="button"
      className={`record-chip record-chip-${kind}`}
      aria-pressed={pressed}
      onClick={onPress}
    >
      {kind === "meeting" ? (
        <span className="record-chip-mark" aria-hidden="true">
          <CalendarClock size={13} />
        </span>
      ) : (
        // The label names the chip; the initials would only say it twice.
        <span className="record-chip-avatar" aria-hidden="true">
          <Avatar name={label} identity={identity} />
        </span>
      )}
      <span className="record-chip-label">{label}</span>
    </button>
  );
}
