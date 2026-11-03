// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { CSSProperties, ReactNode } from "react";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { Badge } from "./atoms";
import { Heading } from "./heading";
import { Panel } from "./panel";
import { type ReadingTone, readingColours } from "./readingtone";
import "./howitstands.css";

type WashVars = CSSProperties & Record<`--${string}`, string>;

/**
 * Who wrote the verdict. `records` is a rule over the CRM's own rows, which is
 * every record type but one. `model` is a sentence a model wrote, and only then
 * does the card wear the indigo sources pill.
 */
export type StandingSources =
  | Readonly<{ by: "records" }>
  | Readonly<{ by: "model"; count: number }>;

/** The call at the head of the card: one word, the sentence under it, and
 *  the tone both are drawn in. */
export type StandingVerdict = Readonly<{
  word: string;
  sentence?: ReactNode;
  tone: ReadingTone;
}>;

/**
 * How it stands: the record's verdict over its readings. A ring sits beside
 * the dimension rows (`heroLayout="beside"`); a stage ladder runs the full
 * width under them (`"below"`) and may keep a `ring` beside the rows. The
 * timeline follows, and the figures close the card edge to edge. Every slot
 * left out is left out of the card.
 */
export function HowItStands({
  date,
  sources = { by: "records" },
  verdict,
  hero,
  heroLayout = "beside",
  ring,
  dims,
  timeline,
  figures,
}: Readonly<{
  // The day the reading describes, formatted by the caller in the record's
  // zone ("13 Aug").
  date: string;
  sources?: StandingSources;
  verdict: StandingVerdict;
  hero?: ReactNode;
  heroLayout?: "beside" | "below";
  // A ring beside the rows while the hero is a ladder below them.
  ring?: ReactNode;
  // `StandingDims`.
  dims?: ReactNode;
  // `StandingTimeline`.
  timeline?: ReactNode;
  // A `StatStrip density="plate"` of `StatCard`s.
  figures?: ReactNode;
}>) {
  const t = useT();
  const colours = readingColours(verdict.tone);
  const besideRing = heroLayout === "beside" ? hero : ring;
  const vars: WashVars = {
    "--standing-wash": colours.wash,
    "--standing-ink":
      verdict.tone === "neutral" ? "var(--textPrimary)" : colours.ink,
  };
  // An unjudged verdict keeps a plain edge, as it keeps a plain ground.
  if (verdict.tone !== "neutral") vars["--standing-edge"] = colours.ink;
  return (
    <div className="standing-sheet" style={vars}>
      <Panel
        className={
          figures
            ? "standing-sheet-pane standing-sheet-pane-plated"
            : "standing-sheet-pane"
        }
      >
        <div className="standing-sheet-head">
          <Heading size="xsmall" as="h2" className="standing-sheet-title">
            {t("standing.title", { date })}
          </Heading>
          <SourcesPill sources={sources} />
        </div>
        <div
          className={
            besideRing
              ? "standing-sheet-hero standing-sheet-hero-ring"
              : "standing-sheet-hero"
          }
        >
          <div className="standing-sheet-verdict">
            <span className="standing-sheet-word">{verdict.word}</span>
            {verdict.sentence ? (
              <p className="standing-sheet-sentence">{verdict.sentence}</p>
            ) : null}
          </div>
          {(dims || besideRing) && (
            <div className="standing-sheet-readings">
              {besideRing}
              {dims}
            </div>
          )}
        </div>
        {hero && heroLayout === "below" && hero}
        {timeline}
        {figures && <div className="standing-sheet-figures">{figures}</div>}
      </Panel>
    </div>
  );
}

function SourcesPill({ sources }: Readonly<{ sources: StandingSources }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (sources.by === "model") {
    return (
      <Badge tone="ai">
        {plural("standing.sources.model", sources.count, {
          count: formatNumber(sources.count, locale),
        })}
      </Badge>
    );
  }
  return <Badge>{t("standing.sources.records")}</Badge>;
}
