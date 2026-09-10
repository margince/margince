// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The queue's headings: the bands the server declared, and inside each one the
// runs of work due on different days.
//
// Split out of worklist.tsx because it is one piece of drawing with two nested
// groupings and its own rules about which of them earn a heading — and because
// the screen it came from is under a length ratchet that only shrinks.

import type { ReactNode } from "react";
import { Eyebrow } from "../design-system/eyebrow";
import { useT } from "../i18n";
import { type BandSection, dueRunHeading, dueRuns } from "./worklist.bands";
import type { WorklistItem } from "./worklist.queries";

/**
 * One band, its heading, and the rows beneath it.
 *
 * A band holding nothing says so in words rather than drawing a heading over a
 * gap — but only when the page can honestly claim it is empty, which is what
 * `canReportEmpty` carries: a page that stopped at its first read does not know
 * whether the band is clear or merely unread.
 */
export function QueueBand<RowProps>({
  section,
  canReportEmpty,
  rows: Rows,
  rowProps,
}: Readonly<{
  section: BandSection;
  canReportEmpty: boolean;
  // The row list is passed IN rather than imported: it lives in the screen
  // beside the props it needs, and reaching back for it would make this file
  // and that one import each other.
  // The row list is passed IN rather than imported: it lives in the screen
  // beside the props it needs, and reaching back for it would make this file
  // and that one import each other.
  rows: (props: RowProps & { items: readonly WorklistItem[] }) => ReactNode;
  rowProps: RowProps;
}>) {
  const t = useT();
  if (section.items.length === 0) {
    if (!canReportEmpty) {
      return null;
    }
    return (
      <div className="worklist-queue-band">
        <Eyebrow as="h3" className="worklist-band">
          {t(`worklist.band.${section.band}` as const)}
        </Eyebrow>
        {/* Said, not left blank. A heading with nothing under it reads as a
            page that failed to draw. */}
        <p className="t-body worklist-band-clear">
          {t(`worklist.bandClear.${section.band}` as const)}
        </p>
      </div>
    );
  }
  return (
    <div className="worklist-queue-band">
      <Eyebrow as="h3" className="worklist-band">
        {t(`worklist.band.${section.band}` as const)}
      </Eyebrow>
      {/* Work due later carries a sub-heading of its own, so a reader can tell
          tomorrow's deadline from today's without reading every date. Overdue
          and today's draw under the band heading, which already says what they
          are. */}
      {dueRuns(section.items).map((run, index) => (
        <div key={run.group ?? `now-${index}`}>
          {run.group && (
            <Eyebrow as="h4" className="worklist-due-run">
              {t(dueRunHeading(run.group))}
            </Eyebrow>
          )}
          <Rows items={run.items} {...rowProps} />
        </div>
      ))}
    </div>
  );
}
