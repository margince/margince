// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";
import type { components } from "../../api/schema";
import { Eyebrow } from "../../design-system/eyebrow";
import { usePrefersReducedMotion } from "../../design-system/motion";
import { formatNumber } from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { bandLabelKeys } from "./narration";
import type { CorpusManifestEntry } from "./use-voice-corpus";
import { emphasisIndices } from "./voice-excerpts";

// The distilling panel: while the reader adds writing, the room reads it back
// — a running line of their own sentences, and between them what the server
// has so far heard in the corpus. Every "hears" line is a fact the summary
// already states (words, sources, registers, quality band); the sentences are
// the reader's own, chosen by voice-excerpts.ts. Nothing here is inferred on
// the client: a panel that guessed at tone before the build ran would be
// inventing the finding the build exists to make.
//
// Decorative on purpose (`aria-hidden`): the same numbers stand in the meter
// and the sources list as real text, and a ticker read aloud every few
// seconds would talk over the reader adding their next file.

type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];

type DistillItem =
  | Readonly<{ kind: "evidence"; text: string }>
  | Readonly<{ kind: "hears"; text: string }>;

// How long each line stays before the next arrives, and how many stay on
// screen. Slow enough to be read as prose, few enough that the panel stays a
// glance rather than a transcript.
const STEP_MS = 2600;
const VISIBLE = 5;

// The registers a corpus can carry, in the summary's own keys. A register the
// server adds later shows under its wire name rather than vanishing.
const REGISTER_LABELS: Readonly<Record<string, string>> = {
  general: "general",
  spoken: "spoken",
};

function hearsLines(
  summary: CorpusSummary,
  t: Translator,
  locale: Locale,
): string[] {
  const lines = [
    t("ob.conv.voice.hearsWords", {
      words: formatNumber(summary.total_words, locale),
      sources: formatNumber(summary.source_count, locale),
    }),
    t("ob.conv.voice.hearsBand", {
      band: t(bandLabelKeys[summary.quality_band]),
    }),
  ];
  for (const [register, words] of Object.entries(summary.register_words)) {
    if (words > 0) {
      lines.push(
        t("ob.conv.voice.hearsRegister", {
          register: REGISTER_LABELS[register] ?? register,
          words: formatNumber(words, locale),
        }),
      );
    }
  }
  return lines;
}

// Two sentences, then a finding, round and round: the reader's words carry
// the panel and the server's readings punctuate them.
function sequence(evidence: readonly string[], hears: readonly string[]) {
  const items: DistillItem[] = [];
  let heard = 0;
  evidence.forEach((text, index) => {
    items.push({ kind: "evidence", text });
    if (index % 2 === 1 && hears.length > 0) {
      items.push({ kind: "hears", text: hears[heard % hears.length] });
      heard += 1;
    }
  });
  if (evidence.length === 0) {
    for (const text of hears) {
      items.push({ kind: "hears", text });
    }
  }
  return items;
}

/**
 * The index of the newest visible item, advancing on a timer and wrapping.
 * Timer-driven rather than rAF: a tab the reader left to find their next file
 * must still be moving when they come back. Under reduced motion the panel
 * holds its first screen and never advances.
 */
function useTicker(length: number): number {
  const reduced = usePrefersReducedMotion();
  const [ticks, setTicks] = useState(0);
  useEffect(() => {
    if (reduced || length <= VISIBLE) {
      return;
    }
    const timer = setInterval(() => setTicks((prev) => prev + 1), STEP_MS);
    return () => clearInterval(timer);
  }, [length, reduced]);
  return length === 0 ? 0 : (Math.min(VISIBLE, length) - 1 + ticks) % length;
}

function EvidenceLine({ text }: Readonly<{ text: string }>) {
  const words = text.split(" ");
  const lit = emphasisIndices(words);
  // A sentence may repeat a word, so each occurrence is keyed by which
  // repetition it is — stable across renders of the same line, and never a
  // bare position.
  const seen = new Map<string, number>();
  return (
    <p className="ob-distill-evidence">
      {words.map((word, index) => {
        const occurrence = seen.get(word) ?? 0;
        seen.set(word, occurrence + 1);
        return (
          <span key={`${word}#${occurrence}`}>
            {lit.has(index) ? <mark>{word}</mark> : word}
            {index < words.length - 1 ? " " : ""}
          </span>
        );
      })}
    </p>
  );
}

export function VoiceDistillPanel({
  manifest,
  summary,
}: Readonly<{
  manifest: readonly CorpusManifestEntry[];
  summary: CorpusSummary | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const evidence = manifest.flatMap((entry) => entry.lines);
  const items = sequence(
    evidence,
    summary === null ? [] : hearsLines(summary, t, locale),
  );
  const head = useTicker(items.length);
  if (items.length === 0) {
    return null;
  }
  // The window ending at `head`, oldest first, wrapping around the sequence
  // so the panel never goes blank between the last item and the first.
  const count = Math.min(VISIBLE, items.length);
  const shown = Array.from({ length: count }, (_, i) => {
    const index = (head - (count - 1) + i + items.length) % items.length;
    return { item: items[index], index };
  });
  return (
    <aside className="ob-distill" aria-hidden="true">
      <Eyebrow className="ob-distill-eyebrow">
        <i className="ob-distill-pulse" /> {t("ob.conv.voice.distilling")}
      </Eyebrow>
      <div className="ob-distill-feed">
        {shown.map(({ item, index }, position) => (
          <div
            key={index}
            className="ob-distill-item"
            data-kind={item.kind}
            // How many lines have arrived since this one: the stylesheet
            // fades the older ones, newest fully lit at the bottom.
            data-age={shown.length - 1 - position}
          >
            {item.kind === "evidence" ? (
              <EvidenceLine text={item.text} />
            ) : (
              <p className="ob-distill-hears">
                <b>{t("ob.conv.voice.hears")}</b> {item.text}
              </p>
            )}
          </div>
        ))}
      </div>
    </aside>
  );
}
