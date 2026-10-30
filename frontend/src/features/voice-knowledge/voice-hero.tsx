import { ArrowDown, Plus } from "lucide-react";
import type { components } from "../../api/schema";
import { Voiceprint } from "../../components/voiceprint";
import { Button } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { useJumpTo } from "../../design-system/jumpbar";
import { Meter } from "../../design-system/readings";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { type VoiceInsightsData, VoicePortrait } from "./voice-insights";
import { bandLabel } from "./voice-samples";
import "./voice-dna.css";

type VoiceProfile = components["schemas"]["VoiceProfile"];
type VoiceCorpusSummary = components["schemas"]["VoiceCorpusSummary"];

/** The ids the page's sections carry, so the jump bar and the hero's verbs
 * land on the same places. */
export const VOICE_SECTIONS = {
  voice: "vdna-voice",
  moves: "vdna-moves",
  drafts: "vdna-drafts",
  sharpen: "vdna-sharpen",
  rules: "vdna-rules",
  builds: "vdna-builds",
} as const;

const BANDS = ["thin", "good", "rich", "sharp"] as const;

// The head of the page: who the writer is, beside the voiceprint whose ring is
// the corpus toward its target. Before the first build the portrait is the
// promise of one, and the verb is to add writing.
export function VoiceHero({
  profile,
  insights,
  profileVersion,
  summary,
  hasDrafts,
}: Readonly<{
  profile: VoiceProfile;
  insights: VoiceInsightsData | null;
  profileVersion: number;
  summary: VoiceCorpusSummary | undefined;
  hasDrafts: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const jump = useJumpTo();
  const total = summary?.total_words ?? 0;
  const target = summary?.target_words ?? 0;
  const band = summary?.quality_band ?? profile.quality_band;
  const meter = t("settings.voice.meter", {
    count: formatNumber(total, locale),
    target: formatNumber(target, locale),
  });
  const strength = t("settings.voice.strength", { band: bandLabel(t, band) });
  return (
    <section
      id={VOICE_SECTIONS.voice}
      tabIndex={-1}
      className="vdna-hero"
      aria-label={t("settings.voice.heroLabel")}
    >
      <div className="vdna-hero-text">
        {insights ? (
          <VoicePortrait data={insights} profileVersion={profileVersion} />
        ) : (
          <div className="vdna-portrait">
            <Heading size="large" as="h2" className="vdna-identity">
              {t("settings.voice.heroPending")}
            </Heading>
            <p className="vdna-thinking">
              {t("settings.voice.heroPendingBody")}
            </p>
          </div>
        )}
        <div className="vdna-hero-acts">
          {hasDrafts && (
            <Button onClick={() => jump(VOICE_SECTIONS.drafts)}>
              {t("settings.voice.jump.drafts")}
            </Button>
          )}
          <Button onClick={() => jump(VOICE_SECTIONS.rules)}>
            {t("settings.voice.editRules")}
          </Button>
        </div>
      </div>
      {summary && (
        <div className="vdna-print">
          <Voiceprint
            fraction={target > 0 ? total / target : 0}
            amplitude={insights ? 1 : 0.4}
            label={`${strength}: ${meter}`}
          />
          <div className="vdna-print-label">
            <span className="vdna-print-band">{strength}</span>
            <span className="t-caption">{meter}</span>
          </div>
          <ol className="vdna-ladder" aria-hidden="true">
            {BANDS.map((step) => (
              <li key={step} data-current={step === band || undefined}>
                {bandLabel(t, step)}
              </li>
            ))}
          </ol>
        </div>
      )}
    </section>
  );
}

// The one move that makes the voice better, kept at the top of the page: how
// far the corpus is from its target, what to add next, and the way to add it.
export function SharpenStrip({
  summary,
  next,
  canEdit,
}: Readonly<{
  summary: VoiceCorpusSummary;
  next: string | null;
  canEdit: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const jump = useJumpTo();
  const left = Math.max(0, summary.target_words - summary.total_words);
  const headline =
    left > 0
      ? plural("settings.voice.toSharp", left, {
          count: formatNumber(left, locale),
        })
      : t("settings.voice.atSharp");
  return (
    <div className="vdna-sharpen">
      <div className="vdna-sharpen-text">
        <b>{headline}</b>
        <Meter
          dense
          flat
          value={summary.total_words}
          max={Math.max(summary.target_words, 1)}
          label={t("settings.voice.meter", {
            count: formatNumber(summary.total_words, locale),
            target: formatNumber(summary.target_words, locale),
          })}
        />
        {next && <span className="t-caption">{next}</span>}
      </div>
      <div className="vdna-sharpen-acts">
        {canEdit && (
          <Button
            variant="primary"
            onClick={() => jump(VOICE_SECTIONS.sharpen)}
          >
            <Plus aria-hidden /> {t("settings.voice.addWriting")}
          </Button>
        )}
        <Button onClick={() => jump(VOICE_SECTIONS.sharpen)}>
          <ArrowDown aria-hidden /> {t("settings.voice.seeSamples")}
        </Button>
      </div>
    </div>
  );
}
