// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// THE LEAD BRIEF: the call the page opens on, in the record-360 kit's own
// shape (the tinted card, the loud standing, the sentence it rests on and
// the disclosure behind it), but named for itself rather than borrowed from
// `CallCard`.
//
// A lead earns no "<name> · 360" claim the way a company or a deal's call
// does: `leadStanding` reads the ladder and the clocks live, on every render,
// from facts already on the record, rather than a model's dated prose. The
// sparkle-and-subject head (`BriefTitle`) says "a machine wrote what is under
// this", a claim this card cannot make honestly, and a dated "Last update"
// line and `WrittenBy` badge would name an author for a reading nobody
// authored. So this card states its own plain title instead of `CallCard`'s,
// and carries neither.
//
// Kept apart from `CallCard`/`reading.tsx` rather than adding a lead branch to
// it: that file is shared with the company and the deal call, and a title this
// card owes but they do not is this card's own to draw.

import type { ReactNode } from "react";
import { Panel } from "../design-system/panel";
import { useT } from "../i18n";
import { type Grounding, type StandingTone, VerdictHead } from "./record360";
import "./company360.css";

/**
 * LeadBrief is the lead's own call: the standing the ladder and the
 * first-response clock reached, the sentence it rests on, and under them
 * whatever the call was read from, the lead's own thread.
 *
 * `standing` is absent only while the caller has nothing to show yet; every
 * lead reaches a standing by construction (`leadStanding` is total over the
 * five states), so this stays as defensive as `CallCard`'s own optional prop.
 */
export function LeadBrief({
  standing,
  because,
  restsOn,
  children,
}: Readonly<{
  standing?: { label: string; tone: StandingTone };
  // One line saying what the call rests on: the half a scanner reads.
  because?: ReactNode;
  // The readings behind the call, one disclosure away.
  restsOn?: readonly Grounding[];
  children?: ReactNode;
}>) {
  const t = useT();
  return (
    <Panel tone="ai" title={t("lead.brief.title")}>
      {standing ? (
        <VerdictHead
          label={standing.label}
          tone={standing.tone}
          because={because}
          restsOn={restsOn}
          scale="compact"
        />
      ) : null}
      {children}
    </Panel>
  );
}
