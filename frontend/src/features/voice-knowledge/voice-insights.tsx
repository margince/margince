import { ArrowRight, Ban, FileText, Lightbulb } from "lucide-react";
import { useState } from "react";
import type { components } from "../../api/schema";
import { Voiceprint } from "../../components/voiceprint";
import { Badge, Card, SegmentedControl } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber, identifierNumber } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import "./voice-dna.css";

type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];
type VoiceLearningSummary = components["schemas"]["VoiceLearningSummary"];

// The derived artifact's structured half (profile_json/stats_json) is
// free-form JSON on the wire; these narrowing helpers keep the screen
// honest about what the builder actually stored — a missing or malformed
// section renders as absent, never as a crash or an invented value.
function asRecord(value: unknown): Record<string, unknown> | null {
  if (typeof value === "object" && value !== null && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  return null;
}

function asString(value: unknown): string | null {
  return typeof value === "string" && value.trim() !== "" ? value : null;
}

function asNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function asStringList(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const out: string[] = [];
  for (const item of value) {
    const text = asString(item);
    if (text) {
      out.push(text);
    }
  }
  return out;
}

export type VoiceSignatureMove = { move: string; quote: string };
export type VoiceSampleDraft = {
  subject: string;
  body: string;
  score: number | null;
};

export type VoiceInsightsData = {
  identity: string | null;
  thinking: string | null;
  obsessions: string[];
  moves: VoiceSignatureMove[];
  avoid: string[];
  sampleDrafts: VoiceSampleDraft[];
  nextBest: string | null;
  nextBestKey: string | null;
  nextBestWords: number | null;
  words: number | null;
  meanSentence: number | null;
  sources: number | null;
  modelName: string | null;
};

function parseMoves(value: unknown): VoiceSignatureMove[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const moves: VoiceSignatureMove[] = [];
  for (const raw of value) {
    const record = asRecord(raw);
    const move = asString(record?.move);
    const quote = asString(record?.quote);
    if (move && quote) {
      moves.push({ move, quote });
    }
  }
  return moves;
}

function parseSampleDrafts(value: unknown): VoiceSampleDraft[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const drafts: VoiceSampleDraft[] = [];
  for (const raw of value) {
    const record = asRecord(raw);
    const body = asString(record?.body);
    if (body) {
      drafts.push({
        subject: asString(record?.subject) ?? "",
        body,
        score: asNumber(record?.voice_score),
      });
    }
  }
  return drafts;
}

// parseVoiceInsights extracts the presentation payload one active or
// candidate version carries. Everything is optional by construction.
export function parseVoiceInsights(
  version: VoiceProfileVersion,
): VoiceInsightsData {
  const profileJSON = asRecord(version.profile_json) ?? {};
  const statsJSON = asRecord(version.stats_json) ?? {};
  const inference = asRecord(profileJSON.inference) ?? {};
  const guidance = asRecord(profileJSON.guidance) ?? {};
  const moves = parseMoves(inference.signature_moves);
  const sampleDrafts = parseSampleDrafts(profileJSON.sample_drafts);
  return {
    identity: asString(inference.identity_summary),
    thinking: asString(inference.thinking_pattern),
    obsessions: asStringList(inference.observed_obsessions),
    moves,
    avoid: asStringList(inference.avoid),
    sampleDrafts,
    nextBest: asString(guidance.next_best),
    nextBestKey: asString(guidance.next_best_key),
    nextBestWords: asNumber(guidance.next_best_words),
    words: asNumber(statsJSON.word_count),
    meanSentence: asNumber(statsJSON.mean_sentence_words),
    sources: asNumber(statsJSON.sample_count),
    modelName: asString(version.model_name),
  };
}

// nextBestCopy localizes the guidance nudge from its structured key; an
// unknown key (a newer server) falls back to the server's prose.
function nextBestCopy(
  t: ReturnType<typeof useT>,
  data: VoiceInsightsData,
  locale: Locale,
): string | null {
  switch (data.nextBestKey) {
    case "add_transcript":
      return t("voice.insights.next.addTranscript");
    case "add_email":
      return t("voice.insights.next.addEmail");
    case "add_words":
      return t("voice.insights.next.addWords", {
        count: formatNumber(data.nextBestWords ?? 0, locale),
      });
    case "at_target":
      return t("voice.insights.next.atTarget");
    default:
      return null;
  }
}

// nextBestLine is the guidance nudge in the reader's language, or the server's
// own prose when the key is one this client does not know.
export function useNextBestLine(data: VoiceInsightsData | null): string | null {
  const t = useT();
  const { locale } = useLocale();
  if (!data) {
    return null;
  }
  return nextBestCopy(t, data, locale) ?? data.nextBest;
}

// VoiceInsights is the whole impress surface in one column, for the place that
// shows a build before anyone has accepted it: the candidate review. The
// settings page lays the same parts out as sections of its own.
export function VoiceInsights({
  data,
  profileVersion,
}: Readonly<{ data: VoiceInsightsData; profileVersion: number }>) {
  const t = useT();
  const next = useNextBestLine(data);
  return (
    <div className="vdna-insights">
      <VoicePortrait data={data} profileVersion={profileVersion} />
      <VoiceFigures data={data} />
      <VoiceTraits data={data} />
      <DraftShowcase drafts={data.sampleDrafts} />
      {next && (
        <p className="vdna-next">
          <ArrowRight aria-hidden />
          <span>
            <b>{t("voice.insights.nextBestLabel")}</b> {next}
          </span>
        </p>
      )}
    </div>
  );
}

// Who the writer is in one line, set as display type, with where it came from
// above it and how they think under it.
export function VoicePortrait({
  data,
  profileVersion,
}: Readonly<{ data: VoiceInsightsData; profileVersion: number }>) {
  const t = useT();
  return (
    <div className="vdna-portrait">
      <p className="vdna-provenance">
        <Badge tone="ai">
          {t("voice.insights.provenance", {
            n: identifierNumber(profileVersion),
          })}
        </Badge>
        {data.modelName && data.modelName !== "unrecorded" && (
          <span className="t-caption">{data.modelName}</span>
        )}
      </p>
      {data.identity && <p className="vdna-identity">{data.identity}</p>}
      {data.thinking && (
        <p className="vdna-thinking">
          <Lightbulb aria-hidden />
          <span>
            <b>{t("voice.insights.thinkingLabel")}</b> {data.thinking}
          </span>
        </p>
      )}
    </div>
  );
}

// The counts the portrait rests on, each through the locale's own number
// format: a figure handed to `t` straight would print 12.5 to a German reader.
// The learning summary adds how many of its drafts the writer kept.
export function VoiceFigures({
  data,
  learning,
}: Readonly<{ data: VoiceInsightsData; learning?: VoiceLearningSummary }>) {
  const t = useT();
  const { locale } = useLocale();
  const figures = [
    { key: "words", label: t("voice.insights.figWords"), value: data.words },
    {
      key: "sources",
      label: t("voice.insights.figSources"),
      value: data.sources,
    },
    {
      key: "sentence",
      label: t("voice.insights.figSentence"),
      value: data.meanSentence,
    },
  ].filter((figure) => figure.value !== null);
  // Nothing drafted yet is no reading of what was kept, not a zero.
  const kept =
    learning && learning.drafted > 0
      ? learning.accepted + learning.edited_sent
      : null;
  if (figures.length === 0 && kept === null) {
    return null;
  }
  return (
    <StatStrip density="compact">
      {figures.map((figure) => (
        <StatCard
          key={figure.key}
          label={figure.label}
          value={formatNumber(figure.value ?? 0, locale)}
        />
      ))}
      {learning && kept !== null && (
        <StatCard
          label={t("voice.insights.figKept")}
          value={t("voice.insights.figKeptValue", {
            kept: formatNumber(kept, locale),
            drafted: formatNumber(learning.drafted, locale),
          })}
          detail={t("voice.insights.figKeptDetail", {
            accepted: formatNumber(learning.accepted, locale),
            edited: formatNumber(learning.edited_sent, locale),
          })}
          meter={{ filled: kept, total: learning.drafted }}
        />
      )}
    </StatStrip>
  );
}

// What the build learned, in the writer's own words. Up to three quotes show a
// move each, then come the themes and what they avoid.
export function VoiceTraits({ data }: Readonly<{ data: VoiceInsightsData }>) {
  const t = useT();
  return (
    <div className="vdna-traits">
      {data.moves.length > 0 && (
        <ul className="vdna-moves" aria-label={t("voice.insights.movesLabel")}>
          {data.moves.slice(0, 3).map((move) => (
            <li key={move.move}>
              <figure className="vdna-move">
                <blockquote className="vdna-quote">{move.quote}</blockquote>
                <figcaption>
                  <ArrowRight aria-hidden />
                  {move.move}
                </figcaption>
              </figure>
            </li>
          ))}
        </ul>
      )}
      {(data.obsessions.length > 0 || data.avoid.length > 0) && (
        <div className="vdna-pair">
          {data.obsessions.length > 0 && (
            <section>
              <Heading size="xsmall" as="h3" className="vdna-label">
                {t("voice.insights.themesLabel")}
              </Heading>
              <div className="vdna-chips">
                {data.obsessions.map((theme) => (
                  <Badge key={theme}>{theme}</Badge>
                ))}
              </div>
            </section>
          )}
          {data.avoid.length > 0 && (
            <section>
              <Heading size="xsmall" as="h3" className="vdna-label">
                {t("voice.insights.avoidLabel")}
              </Heading>
              <div className="vdna-chips">
                {data.avoid.map((rule) => (
                  <Badge key={rule} icon={Ban}>
                    {rule}
                  </Badge>
                ))}
              </div>
            </section>
          )}
        </div>
      )}
    </div>
  );
}

// The build's sample drafts, one at a time behind a switch. Each shows as the
// email it would be, with its voice match as a ring.
export function DraftShowcase({
  drafts,
}: Readonly<{ drafts: VoiceSampleDraft[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const [picked, setPicked] = useState("0");
  const options = drafts.map((_, index) => String(index));
  const shown = drafts[Number(picked)] ?? drafts[0];
  if (!shown) {
    return null;
  }
  const labels = Object.fromEntries(
    drafts.map((draft, index) => [
      String(index),
      draft.subject ||
        t("voice.insights.draftUntitled", {
          n: formatNumber(index + 1, locale),
        }),
    ]),
  );
  const pct =
    shown.score === null
      ? null
      : formatNumber(Math.round(shown.score * 100), locale);
  return (
    <div className="vdna-drafts">
      {drafts.length > 1 && (
        <SegmentedControl
          options={options}
          value={options.includes(picked) ? picked : "0"}
          onChange={setPicked}
          labels={labels}
          label={t("voice.insights.samplesLabel")}
        />
      )}
      <div className="vdna-draft">
        <Card as="article" inset className="vdna-letter">
          <div className="vdna-letter-head">
            <FileText aria-hidden />
            {shown.subject && <b>{shown.subject}</b>}
            <Badge tone="ai">{t("voice.insights.draftOnly")}</Badge>
          </div>
          <p className="vdna-letter-body">{shown.body}</p>
          <p className="t-caption vdna-disclosure">
            {t("voice.insights.disclosure")}
          </p>
        </Card>
        {pct !== null && shown.score !== null && (
          <div className="vdna-match">
            <Voiceprint
              size="medium"
              fraction={shown.score}
              label={t("voice.insights.voiceScore", { pct })}
            >
              <span className="vdna-match-pct">{pct}%</span>
            </Voiceprint>
            <span className="t-caption">
              {t("voice.insights.matchCaption")}
            </span>
          </div>
        )}
      </div>
    </div>
  );
}
