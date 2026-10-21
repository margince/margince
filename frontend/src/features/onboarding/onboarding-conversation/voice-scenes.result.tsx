import { Lightbulb } from "lucide-react";
import type { ReactNode } from "react";
import { useState } from "react";
import { Button } from "../../../design-system/atoms";
import { formatNumber } from "../../../format/format";
import { type Locale, useLocale, useT } from "../../../i18n";
import type { VoiceInsightsData } from "../../voice-knowledge/voice-insights";
import { parseVoiceInsights } from "../../voice-knowledge/voice-insights";
import type { VoiceProfileVersion } from "./voice-scenes";
import { WayOnward } from "./way-onward";

export { VoiceBuildScene } from "./voice-scenes.build";

/** The scene frame: the body, plus a slot beside the hoisted headline.
 * `wide` opens the frame to the board's full measure for a scene with a second
 * column (the collect scene's distilling panel). The others read best at prose
 * width. */
export function VoiceScene({
  aside,
  wide = false,
  children,
}: Readonly<{
  aside?: ReactNode;
  wide?: boolean;
  children: ReactNode;
}>) {
  return (
    <div
      className={
        wide
          ? "ob-scene ob-voice-scene ob-voice-scene-wide"
          : "ob-scene ob-voice-scene"
      }
    >
      {aside !== undefined && <div className="ob-decision-head">{aside}</div>}
      {children}
    </div>
  );
}

/**
 * The result scene: what a succeeded build learned, as a two-column board. The
 * sample it would send is on the left, the reading behind it on the right. The
 * two answers sit on the pinned rail of every scene's primary action. "That is
 * me" confirms. To disagree, the reader adds more writing and builds again: the
 * one correction the product can act on, unlike a tone word it cannot apply.
 */
export function VoiceResultScene({
  loading,
  version,
  onContinue,
  onRevise,
}: Readonly<{
  loading: boolean;
  version: VoiceProfileVersion | null;
  onContinue: () => void;
  /** Back to collecting, corpus kept, for a build that does not sound like
   * the reader. */
  onRevise: () => void;
}>) {
  const t = useT();
  const candidate = version !== null && version.status === "candidate";
  const data = version !== null ? parseVoiceInsights(version) : null;
  return (
    <VoiceScene>
      {loading && (
        <p className="ob-conv-artifact-empty">
          {t("ob.conv.voice.resultLoading")}
        </p>
      )}
      {!loading && version === null && (
        <p className="ob-conv-artifact-empty">
          {t("ob.conv.voice.resultEmpty")}
        </p>
      )}
      {data !== null && <VoiceResultBoard data={data} />}
      <WayOnward
        label={t("ob.conv.voice.resultContinue")}
        stillNeeded={(why) => why.join(" ")}
        note={
          candidate ? (
            <p className="ob-stage-hint" role="status">
              {t("ob.conv.voice.candidateNote")}
            </p>
          ) : undefined
        }
        onGo={onContinue}
      >
        <Button variant="ghost" onClick={onRevise}>
          {t("ob.conv.voice.revise")}
        </Button>
      </WayOnward>
    </VoiceScene>
  );
}

// The sample: a real draft in a card of its own. `drafts` is every sample
// the build kept. "Another scenario" cycles them locally, since all are in
// hand, and hides when only one is left. The header shows only Subject:
// `VoiceSampleDraft` has no To/From.
function VoiceSampleCard({
  drafts,
  why,
}: Readonly<{
  drafts: VoiceInsightsData["sampleDrafts"];
  why: string;
}>) {
  const t = useT();
  const [index, setIndex] = useState(0);
  const sample = drafts[index % drafts.length];
  if (sample === undefined) {
    return null;
  }
  return (
    <div className="ob-voice-result-card">
      <div className="ob-voice-sample-head">
        <p className="ob-voice-result-label t-eyebrow">
          {t("ob.conv.voice.sampleEyebrow")}
        </p>
        {drafts.length > 1 && (
          <Button
            variant="ghost"
            onClick={() => setIndex((prev) => (prev + 1) % drafts.length)}
          >
            {t("ob.conv.voice.sampleAnother")}
          </Button>
        )}
      </div>
      <p className="ob-voice-sample-subject">
        <span className="ob-voice-sample-field t-eyebrow">
          {t("ob.conv.voice.sampleSubjectLabel")}
        </span>
        <b>{sample.subject}</b>
      </p>
      <p className="ob-voice-sample-body">{sample.body}</p>
      <p className="ob-voice-sample-why t-sub">
        <span className="ob-voice-sample-why-tag t-eyebrow">
          {t("ob.conv.voice.sampleWhyTag")}
        </span>
        {why}
      </p>
    </div>
  );
}

type MeasuredDimension = Readonly<{
  key: string;
  name: string;
  value: string;
  poleLow: string;
  poleHigh: string;
  /** A decorative marker position, 0..1, never itself the measurement.
   * `value` and `evidence` carry every number a reader can trust. */
  fraction: number;
  evidence: string;
}>;

// The bucket edges and the marker's bounds are plain arithmetic on the real
// mean sentence length, never an invented confidence score. Generous bounds
// (6..30) keep the common case away from either end.
const SENTENCE_TERSE_MAX = 12;

const SENTENCE_ELABORATE_MIN = 20;

const SENTENCE_LOW_BOUND = 6;

const SENTENCE_HIGH_BOUND = 30;

function sentenceDimension(
  meanSentence: number,
  t: ReturnType<typeof useT>,
  locale: Locale,
): MeasuredDimension {
  const value =
    meanSentence < SENTENCE_TERSE_MAX
      ? t("ob.conv.voice.dimSentencePoleLow")
      : meanSentence > SENTENCE_ELABORATE_MIN
        ? t("ob.conv.voice.dimSentencePoleHigh")
        : t("ob.conv.voice.dimSentenceMeasured");
  const fraction = Math.max(
    0,
    Math.min(
      1,
      (meanSentence - SENTENCE_LOW_BOUND) /
        (SENTENCE_HIGH_BOUND - SENTENCE_LOW_BOUND),
    ),
  );
  return {
    key: "sentence",
    name: t("ob.conv.voice.dimSentenceName"),
    value,
    poleLow: t("ob.conv.voice.dimSentencePoleLow"),
    poleHigh: t("ob.conv.voice.dimSentencePoleHigh"),
    fraction,
    evidence: t("ob.conv.voice.dimSentenceEvidence", {
      count: formatNumber(meanSentence, locale),
    }),
  };
}

// Sentence length is the only axis `parseVoiceInsights` carries. Formality,
// warmth, directness and vocabulary have no server-measured equivalent, so they
// are absent rather than rendered with an invented marker.
function measuredDimensions(
  data: VoiceInsightsData,
  t: ReturnType<typeof useT>,
  locale: Locale,
): readonly MeasuredDimension[] {
  return data.meanSentence === null
    ? []
    : [sentenceDimension(data.meanSentence, t, locale)];
}

// A readout, not a control: no input, no drag handle, nothing focusable. A
// slider-shaped element a reader cannot move must not look movable.
function VoiceDimensionGauge({ dim }: Readonly<{ dim: MeasuredDimension }>) {
  return (
    <div>
      <div className="ob-voice-dim-head">
        <span className="ob-voice-dim-name">{dim.name}</span>
        <span className="t-sub">{dim.value}</span>
      </div>
      <div className="ob-voice-dim-track" aria-hidden>
        <span
          className="ob-voice-dim-marker"
          style={{ left: `${dim.fraction * 100}%` }}
        />
      </div>
      <div className="ob-voice-dim-poles t-caption" aria-hidden>
        <span>{dim.poleLow}</span>
        <span>{dim.poleHigh}</span>
      </div>
      <p className="ob-voice-dim-evidence t-caption">{dim.evidence}</p>
    </div>
  );
}

function VoiceDimensionsCard({
  dimensions,
  words,
  sources,
}: Readonly<{
  dimensions: readonly MeasuredDimension[];
  words: number | null;
  sources: number | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className="ob-voice-result-card">
      <div className="ob-voice-dims-head">
        <span className="ob-voice-result-label t-eyebrow">
          {t("ob.conv.voice.dimensionsTitle")}
        </span>
        <span className="ob-voice-dims-count t-eyebrow">
          {t("ob.conv.voice.dimensionsCount", {
            count: formatNumber(dimensions.length, locale),
          })}
        </span>
      </div>
      {(words !== null || sources !== null) && (
        <p className="ob-voice-dims-meta t-sub">
          {words !== null &&
            t("voice.insights.statWords", {
              count: formatNumber(words, locale),
            })}
          {words !== null && sources !== null ? " · " : ""}
          {sources !== null &&
            t("voice.insights.statSources", {
              count: formatNumber(sources, locale),
            })}
        </p>
      )}
      <div className="ob-voice-dims-list">
        {dimensions.map((dim) => (
          <VoiceDimensionGauge key={dim.key} dim={dim} />
        ))}
      </div>
    </div>
  );
}

// The reading: the identity summary as the lead line, the thinking pattern
// underneath where the build found one.
function VoiceThinkingCard({
  thinking,
  identity,
}: Readonly<{ thinking: string | null; identity: string | null }>) {
  const t = useT();
  return (
    <div className="ob-voice-result-card">
      {identity !== null && (
        <p className="ob-voice-result-identity">{identity}</p>
      )}
      {thinking !== null && (
        <>
          <p className="ob-voice-result-label t-eyebrow">
            <Lightbulb aria-hidden /> {t("voice.insights.thinkingLabel")}
          </p>
          <p>{thinking}</p>
        </>
      )}
    </div>
  );
}

function VoiceMovesCard({
  moves,
}: Readonly<{ moves: VoiceInsightsData["moves"] }>) {
  const t = useT();
  return (
    <div className="ob-voice-result-card">
      <p className="ob-voice-result-label t-eyebrow">
        {t("voice.insights.movesLabel")}
      </p>
      <ul className="ob-voice-moves">
        {moves.map((move) => (
          <li key={move.move}>
            <b>{move.move}</b>
            <blockquote>{move.quote}</blockquote>
          </li>
        ))}
      </ul>
    </div>
  );
}

function VoiceAvoidCard({ avoid }: Readonly<{ avoid: readonly string[] }>) {
  const t = useT();
  return (
    <div className="ob-voice-result-card">
      <p className="ob-voice-result-label t-eyebrow">
        {t("voice.insights.avoidLabel")}
      </p>
      <ul className="ob-voice-avoid">
        {avoid.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
    </div>
  );
}

/**
 * What a succeeded build learned, as the two-column board: the sample on the
 * left, everything measured or observed on the right. Every fact comes from
 * `parseVoiceInsights`; this renders that data and never parses it again. A
 * section with nothing to show renders nothing. A skipped stage gets no empty
 * card, and a dimension this shape lacks gets no invented marker.
 */
function VoiceResultBoard({ data }: Readonly<{ data: VoiceInsightsData }>) {
  const t = useT();
  const { locale } = useLocale();
  const hasThinking = data.thinking !== null || data.identity !== null;
  const dimensions = measuredDimensions(data, t, locale);
  // The sample's "why" line is a short joined phrase of the signature-move
  // names the moves card lists in full. The identity summary leads the
  // thinking card and is not repeated here.
  const why =
    data.moves.length > 0
      ? data.moves.map((move) => move.move).join(", ")
      : t("voice.insights.draftOnly");
  const hasSample = data.sampleDrafts.length > 0;
  return (
    <div
      className={
        hasSample ? "ob-voice-board" : "ob-voice-board ob-voice-board-single"
      }
    >
      {hasSample && (
        <div className="ob-voice-board-col">
          <VoiceSampleCard drafts={data.sampleDrafts} why={why} />
        </div>
      )}
      <div className="ob-voice-board-col">
        {dimensions.length > 0 && (
          <VoiceDimensionsCard
            dimensions={dimensions}
            words={data.words}
            sources={data.sources}
          />
        )}
        {hasThinking && (
          <VoiceThinkingCard
            thinking={data.thinking}
            identity={data.identity}
          />
        )}
        {data.moves.length > 0 && <VoiceMovesCard moves={data.moves} />}
        {data.avoid.length > 0 && <VoiceAvoidCard avoid={data.avoid} />}
        {data.nextBest !== null && (
          <p className="ob-voice-result-next">
            <b>{t("voice.insights.nextBestLabel")}</b> {data.nextBest}
          </p>
        )}
      </div>
    </div>
  );
}
