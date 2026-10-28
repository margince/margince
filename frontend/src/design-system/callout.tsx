// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Sparkles, X } from "lucide-react";
import type { ReactNode } from "react";
import { IconAction } from "./iconaction";
import {
  CheckCircleFilled,
  ErrorFilled,
  type Glyph,
  InfoFilled,
  Lightbulb2Filled,
  WarningFilled,
} from "./materialsymbols";
import "./callout.css";

// Callout: what a surface says about itself, not content. Each tone is a claim,
// so the set is closed. `info` carries no urgency. `warning`: harm if nothing is
// done. `danger`: wrong now, or about to be irreversible. `success`: an action
// landed. `discovery`: new to this reader. `ai`: a machine made what it is about.
export const CALLOUT_TONES = [
  "info",
  "success",
  "warning",
  "danger",
  "discovery",
  "ai",
] as const;

export type CalloutTone = (typeof CALLOUT_TONES)[number];

/**
 * What the notice IS, which is a different question from how loud it is.
 *
 * `outcome` answers something the reader just did — a save failed, an invite
 * landed. `standing` is true as the page renders — a licence in grace, a
 * missing prerequisite, help text. `event` reports something that happened
 * elsewhere or asynchronously — a job finished, a connector broke, work
 * arrived. That is a question a caller can answer about their own screen, and
 * `live` is then derived from it rather than chosen per site by whoever was
 * thinking about ARIA that afternoon.
 */
export type CalloutKind = "outcome" | "standing" | "event";

/**
 * The tone's glyph, so the claim reaches a reader who does not read colour. A
 * filled Material mark, since a tone mark stands alone; `ai` alone keeps lucide.
 */
const TONE_ICONS: Readonly<Record<CalloutTone, Glyph>> = {
  info: InfoFilled,
  warning: WarningFilled,
  danger: ErrorFilled,
  success: CheckCircleFilled,
  // Something the reader has not met yet. Deliberately NOT a Sparkle: that
  // family is the AI mark, and a notice about a new capability drawn in it
  // would claim a machine wrote the notice.
  discovery: Lightbulb2Filled,
  // The product's one mark for "a machine did this", the same glyph the
  // provenance tags and the AI badge wear.
  ai: Sparkles,
};

/** With no `live`, a failed outcome is an alert and other outcomes and events a
 * status. A standing notice arrives with the page, so it announces nothing. */
function announcementFor(
  kind: CalloutKind | undefined,
  tone: CalloutTone,
): "status" | "alert" | undefined {
  if (kind === undefined || kind === "standing") return undefined;
  return kind === "outcome" && tone === "danger" ? "alert" : "status";
}

/** The tone's icon in its ink on a wash of its hue; words and verbs in plain
 * ink. No `className`: a notice wanting its own colour or edge is a second one. */
export function Callout({
  tone = "info",
  kind,
  icon: Icon = TONE_ICONS[tone],
  title,
  actions,
  live,
  dismiss,
  id,
  children,
}: Readonly<{
  tone?: CalloutTone;
  kind?: CalloutKind;
  /**
   * Replaces the tone's glyph, where the notice is about a nameable thing
   * rather than about how bad the news is.
   */
  icon?: Glyph;
  /** The notice's first words, and the one part of it that is not optional. */
  title: ReactNode;
  /** Buttons or links, laid out after the body. */
  actions?: ReactNode;
  live?: "status" | "alert";
  /**
   * The control that puts the notice away, at the end of the card. ONE prop
   * rather than two, so the handler and the word for it cannot arrive apart: an
   * unnamed `X` is the failure `IconAction` exists to prevent, and a label with
   * no handler is a control that does nothing. The label is the caller's
   * translated verb — this file holds no words.
   */
  dismiss?: Readonly<{ label: string; onDismiss: () => void }>;
  /** For a control's `aria-describedby`, when the notice explains that control. */
  id?: string;
  /** Whatever the heading does not already say, where there is more to say. */
  children?: ReactNode;
}>) {
  return (
    <div
      id={id}
      className={`callout callout-${tone}`}
      role={live ?? announcementFor(kind, tone)}
    >
      <span className="callout-icon" aria-hidden="true">
        <Icon size={16} />
      </span>
      <div className="callout-body">
        <div className="callout-copy">
          <p className="callout-title">{title}</p>
          {children !== undefined && (
            <div className="callout-text">{children}</div>
          )}
        </div>
        {actions !== undefined && (
          <div className="callout-actions">{actions}</div>
        )}
      </div>
      {dismiss !== undefined && (
        <span className="callout-dismiss">
          <IconAction
            label={dismiss.label}
            icon={<X aria-hidden />}
            onClick={dismiss.onDismiss}
          />
        </span>
      )}
    </div>
  );
}
