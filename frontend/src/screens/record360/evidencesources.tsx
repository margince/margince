// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The basis under an action, where the words are what the reader acts on.
//
// Citations is the compact form: chips inside a sentence, a reference beside
// prose, everything on one line because the prose is the claim and the chips
// are the receipts. That is right for a brief and wrong here. A suggestion says
// "follow up on the fallback decision" and a recommended move says "they asked
// and nobody answered" — the reader's next act is to READ THE MESSAGE, so the
// message gets a row of its own with the subject, the sender and the preview,
// which is exactly EmailEntry.
//
// It is a separate component rather than a mode on Citations because of where
// each is mounted. Citations renders inline, inside a <p>, an <li> and a
// <span>; EmailEntry is a block row and cannot legally go there. One component
// answering both would be a block element that has to promise never to be a
// block, which is the promise nothing checks.
//
// Two wire shapes feed it. The shared citation carries an account's evidence;
// the deal card's basis is DealNextBestActionEvidence, its own schema with its
// own text field. They are adapted here rather than merged upstream, because
// they are genuinely different rows that happen to name the same message.

import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { EmailEntry } from "../../design-system/emailentry";
import { formatDateTime } from "../../format/format";
import { useLocale } from "../../i18n";
import { type Cited, Citations } from "./citations";
// The basis block is styled beside the citation chips it falls back to, in the
// stylesheet those rules already live in.
import "../company360.css";

type DealMoveEvidence = components["schemas"]["DealNextBestActionEvidence"];

/**
 * One line of an action's basis, in the shape this component draws.
 *
 * `summary` is present when the line rests on a message this reader may have a
 * summary of; `cited` is the same line as a citation, for everything else. A
 * line always has one or the other — a message is drawn as a message, and
 * anything else falls back to the compact citation path.
 */
type Source =
  | { kind: "email"; key: string; summary: NonNullable<Cited["email_summary"]> }
  | { kind: "cited"; key: string; cited: Cited };

/** The shared citation shape as sources. */
export function fromCitations(evidence: readonly Cited[]): Source[] {
  return evidence.map((cited, index) => {
    const summary = cited.entity_type === "activity" ? cited.email_summary : null;
    return summary
      ? { kind: "email", key: `email:${summary.activity_id}`, summary }
      : { kind: "cited", key: `cited:${cited.entity_type}:${cited.entity_id}:${index}`, cited };
  });
}

/**
 * The deal move's own basis as sources.
 *
 * Its non-email lines carry `text` and no record at all — a close date inside
 * the week is a fact about the deal, not a row to open — so they stay the
 * sentence the server wrote rather than becoming a citation with nothing behind
 * it.
 */
export function fromDealMove(evidence: readonly DealMoveEvidence[]): {
  sources: Source[];
  prose: string[];
} {
  const sources: Source[] = [];
  const prose: string[] = [];
  for (const [index, row] of evidence.entries()) {
    if (row.email_summary) {
      sources.push({
        kind: "email",
        key: `email:${row.email_summary.activity_id}:${index}`,
        summary: row.email_summary,
      });
      continue;
    }
    prose.push(row.text);
  }
  return { sources, prose };
}

/**
 * EvidenceSources draws an action's basis as rows.
 *
 * A message this reader may open is a pressable EmailEntry; a withheld one is
 * the same row with its words gone and no opener, which says the exchange
 * happened and its content is not theirs. Everything else goes back to
 * Citations, so a basis mixing a message and a deal renders both correctly
 * without this component learning what a deal is.
 */
export function EvidenceSources({
  sources,
  onOpenEmail,
  onOpenRecord,
}: Readonly<{
  sources: readonly Source[];
  onOpenEmail?: (activityId: string) => void;
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const cited = sources.flatMap((source) =>
    source.kind === "cited" ? [source.cited] : [],
  );
  const messages = sources.flatMap((source) =>
    source.kind === "email" ? [source] : [],
  );
  if (messages.length === 0 && cited.length === 0) {
    return null;
  }
  return (
    <div className="co-evidence-sources">
      {messages.map((source) => {
        const withheld = source.summary.display_status === "withheld";
        const timestamp = formatDateTime(
          source.summary.occurred_at,
          locale,
          recordZone,
        );
        // The withheld branch states its reason rather than passing an
        // undefined opener: EmailEntry refuses to be silently unopenable, and
        // "the content is not this reader's" is a different fact from "this
        // surface mounts no drawer".
        return withheld ? (
          <EmailEntry
            key={source.key}
            summary={source.summary}
            timestamp={timestamp}
            onOpen={undefined}
            whyNotOpenable="withheld"
          />
        ) : (
          <EmailEntry
            key={source.key}
            summary={source.summary}
            timestamp={timestamp}
            {...(onOpenEmail
              ? { onOpen: () => onOpenEmail(source.summary.activity_id) }
              : { onOpen: undefined, whyNotOpenable: "noReader" as const })}
          />
        );
      })}
      {cited.length > 0 && (
        <Citations
          evidence={cited}
          onOpenRecord={onOpenRecord}
          onOpenEmail={onOpenEmail}
        />
      )}
    </div>
  );
}
