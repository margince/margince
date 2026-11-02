import { ArrowDown, Check, Lock, Plus } from "lucide-react";
import { type ReactNode, useId } from "react";
import type { components } from "../../api/schema";
import { Voiceprint } from "../../components/voiceprint";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useJumpTo } from "../../design-system/jumpbar";
import { Meter } from "../../design-system/readings";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import {
  BuildButton,
  useVoiceBuild,
  VOICE_FIRST_BUILD_WORDS,
  type VoiceBuild,
} from "./voice-dna.build";
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
// promise of one, and the verb is to add writing. A first version awaiting
// review is the last step, so its decision takes the place of the verbs.
export function VoiceHero({
  profile,
  insights,
  profileVersion,
  summary,
  hasDrafts,
  canEdit,
  intakeBusy,
  onBuilt,
  review,
}: Readonly<{
  profile: VoiceProfile;
  insights: VoiceInsightsData | null;
  profileVersion: number;
  summary: VoiceCorpusSummary | undefined;
  hasDrafts: boolean;
  canEdit: boolean;
  intakeBusy: boolean;
  onBuilt: () => void;
  review?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
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
        {review ? (
          <div className="vdna-firstbuild">
            <VoiceSteps done={2} />
            {review}
          </div>
        ) : (profile.profile_version ?? 0) > 0 ? (
          <LearnedActs hasDrafts={hasDrafts} />
        ) : (
          <FirstBuild
            profile={profile}
            canEdit={canEdit}
            intakeBusy={intakeBusy}
            onBuilt={onBuilt}
          />
        )}
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

// After a build, the hero's verbs reach what it learned.
function LearnedActs({ hasDrafts }: Readonly<{ hasDrafts: boolean }>) {
  const t = useT();
  const jump = useJumpTo();
  return (
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
  );
}

// Before the first build the hero walks the writer through it. Add writing
// until the server says a build can learn from it, then build, right here.
// The verb that moves the writer on is the one drawn in the AI colour.
function FirstBuild({
  profile,
  canEdit,
  intakeBusy,
  onBuilt,
}: Readonly<{
  profile: VoiceProfile;
  canEdit: boolean;
  intakeBusy: boolean;
  onBuilt: () => void;
}>) {
  const run = useVoiceBuild(profile, onBuilt);
  const reasonId = useId();
  const ready = !run.tooThin;
  return (
    <div className="vdna-firstbuild">
      <VoiceSteps done={ready ? 1 : 0} />
      {!ready && run.words !== null && (
        <Meter
          dense
          tone="ai"
          value={run.words}
          max={VOICE_FIRST_BUILD_WORDS}
          label={run.blocked ?? ""}
        />
      )}
      {canEdit && (
        <FirstBuildActs
          run={run}
          profile={profile}
          intakeBusy={intakeBusy}
          reasonId={reasonId}
        />
      )}
      {run.blocked && (
        <span id={reasonId} className="t-caption">
          {run.blocked}
        </span>
      )}
      {/* Mounted before any outcome: a live region inserted with its text is
          not reliably announced, and a build runs for about a minute. */}
      <p className="t-caption" role="status">
        {run.progress || (ready ? run.reach : null)}
      </p>
      {run.error && <ErrorLine>{run.error}</ErrorLine>}
    </div>
  );
}

// The next step leads: adding writing below the floor, the build above it.
function FirstBuildActs({
  run,
  profile,
  intakeBusy,
  reasonId,
}: Readonly<{
  run: VoiceBuild;
  profile: VoiceProfile;
  intakeBusy: boolean;
  reasonId: string;
}>) {
  const t = useT();
  const jump = useJumpTo();
  const ready = !run.tooThin;
  const build = (
    <BuildButton
      key="build"
      run={run}
      profile={profile}
      intakeBusy={intakeBusy}
      variant={ready ? "ai" : "ghost"}
      describedBy={run.blocked === null ? undefined : reasonId}
    />
  );
  const add = (
    <Button
      key="add"
      variant={ready ? "ghost" : "ai"}
      onClick={() => jump(VOICE_SECTIONS.sharpen)}
    >
      <Plus aria-hidden />{" "}
      {t(ready ? "settings.voice.addMore" : "settings.voice.addWriting")}
    </Button>
  );
  return (
    <div className="vdna-hero-acts">
      {ready ? [build, add] : [add, build]}
      <Button onClick={() => jump(VOICE_SECTIONS.rules)}>
        {t("settings.voice.editRules")}
      </Button>
    </div>
  );
}

const STEPS = [
  { title: "settings.voice.start.add", body: "settings.voice.start.addBody" },
  {
    title: "settings.voice.start.build",
    body: "settings.voice.start.buildBody",
  },
  {
    title: "settings.voice.start.drafts",
    body: "settings.voice.start.draftsBody",
  },
] as const;

// The path to a learned voice, with the steps already taken ticked. A page with
// nothing learned still says how far along the writer is.
function VoiceSteps({ done }: Readonly<{ done: number }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <ol
      className="vdna-steps"
      aria-label={t("settings.voice.start.stepsLabel")}
    >
      {STEPS.map((step, index) => {
        const finished = index < done;
        return (
          <li
            key={step.title}
            data-state={
              finished ? "done" : index === done ? "current" : undefined
            }
          >
            <span className="vdna-step-mark" aria-hidden="true">
              {finished ? <Check /> : formatNumber(index + 1, locale)}
            </span>
            <span className="vdna-step-text">
              <b>{t(step.title)}</b>
              <span className="t-caption">{t(step.body)}</span>
            </span>
          </li>
        );
      })}
    </ol>
  );
}

// The page before any writing exists: the promise of a voice beside a
// voiceprint that has heard nothing yet. The intake sits below it, so this
// head carries no verb of its own.
export function VoiceStart() {
  const t = useT();
  return (
    <section className="vdna-hero" aria-label={t("settings.voice.heroLabel")}>
      <div className="vdna-hero-text">
        <div className="vdna-portrait">
          <Heading size="large" as="h2" className="vdna-identity">
            {t("settings.voice.start.title")}
          </Heading>
          <p className="vdna-thinking">{t("settings.voice.start.body")}</p>
        </div>
        <VoiceSteps done={0} />
        <p className="t-caption vdna-privacy">
          <Lock aria-hidden /> {t("settings.voice.privacy")}
        </p>
      </div>
      <div className="vdna-print">
        <Voiceprint fraction={0} amplitude={0.4} />
        <div className="vdna-print-label">
          <span className="vdna-print-band">
            {t("settings.voice.start.idle")}
          </span>
          <span className="t-caption">
            {t("settings.voice.start.idleBody")}
          </span>
        </div>
      </div>
    </section>
  );
}

// The one move that makes the voice better, at the top of the page. It says
// how far the corpus is from its target and what to add next.
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
          tone="ai"
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
          <Button variant="ai" onClick={() => jump(VOICE_SECTIONS.sharpen)}>
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
