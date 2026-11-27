// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type {
  StandingCitation,
  StandingSources,
} from "../design-system/howitstands";
import { ACTIVITY_KIND_LABEL } from "../design-system/timelineentry";
import { activityDay } from "../format/activitywhen";
import { formatDayMonth } from "../format/format.dates";
import type { Locale, Translator } from "../i18n";
import { labelFor } from "./labelfor";
import type { SourceOpenerFor } from "./opensource";

type StandingLine = components["schemas"]["StandingLine"];
type Activity = components["schemas"]["Activity"];

/** What a citation is named by: the rows the page already read. */
export type CitationWords = Readonly<{
  rows: readonly Activity[] | undefined;
  t: Translator;
  locale: Locale;
  zone: string;
  open: SourceOpenerFor;
}>;

/**
 * The verdict's sentence once a record has a status line: what its newest
 * activity said, written when that activity landed. The line replaces the
 * sentence the page derives, the card says a model wrote it, and each
 * activity it cites opens from under it. Without a line, the page's own
 * reading stands.
 */
export function withStandingLine<V extends { sentence?: unknown }>(
  verdict: V,
  line: StandingLine | undefined,
  words: CitationWords,
): Readonly<{
  verdict: V;
  sources?: StandingSources;
  citations?: readonly StandingCitation[];
}> {
  if (!line || line.text.trim() === "") return { verdict };
  return {
    verdict: { ...verdict, sentence: line.text },
    sources: { by: "model", count: line.activity_ids.length },
    citations: line.activity_ids.map((id) => citation(id, words)),
  };
}

// A cited row the page did not list still opens; it is only named plainly.
function citation(id: string, words: CitationWords): StandingCitation {
  const { rows, t, locale, zone, open } = words;
  const row = rows?.find((candidate) => candidate.id === id);
  if (!row) {
    return {
      id,
      label: t("standing.citations.source"),
      onOpen: open({ id, kind: "note" }),
    };
  }
  const day = activityDay(
    row.occurred_at,
    row.occurred_precision,
    locale,
    zone,
    formatDayMonth,
  );
  return {
    id,
    label: `${t(labelFor(ACTIVITY_KIND_LABEL, row.kind))} · ${day}`,
    onOpen: open({ id, kind: row.kind, contentState: row.content_state }),
  };
}
