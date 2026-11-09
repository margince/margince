// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { writtenAsMarkdown } from "../design-system/activitytimeline";
import { markdownLines } from "../design-system/markdown-text";
import { isTranscriptActivity } from "../design-system/transcriptread";
import { activityAgo } from "../format/activitywhen";
import { emailSummaryText } from "../format/emailtext";
import { useNow } from "../format/now";
import { useLocale, useT } from "../i18n";
import { useViewerZone } from "../lib/viewerzone";

type Activity = components["schemas"]["Activity"];

// The words a reader is shown for the item: what it is about, and the
// sender's own text. A withheld row has neither, and says only what it is.
export function itemWords(activity: Activity): {
  subject?: string;
  excerpt?: string;
} {
  if (activity.content_state === "withheld") {
    return {};
  }
  const summary = activity.email_summary;
  const subject = (summary?.subject ?? activity.subject)?.trim() || undefined;
  const raw = activity.body ?? "";
  const excerpt =
    summary?.preview ??
    (activity.kind === "email"
      ? emailSummaryText(raw)
      : oneLine(activity, raw));
  return { subject, excerpt: excerpt || undefined };
}

// A form's note is one answer per line; the row reads them as one line. Text
// the timeline renders as markdown reads as the text it renders; a chat
// message's characters are its own.
function oneLine(activity: Activity, text: string): string {
  const markdown = writtenAsMarkdown({
    kind: activity.kind,
    transcript: isTranscriptActivity(activity),
  });
  const lines = markdown
    ? markdownLines(text)
    : text.split("\n").map((line) => line.trim());
  return lines.filter(Boolean).join(" · ");
}

export function When({ activity }: Readonly<{ activity: Activity }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  // A minute is the finest unit the label shows.
  const now = useNow(60_000);
  const moment = {
    at: activity.occurred_at,
    precision: activity.occurred_precision,
  };
  return (
    <time dateTime={activity.occurred_at} className="latest-when">
      {activityAgo(moment, now, t, locale, zone)}
    </time>
  );
}
