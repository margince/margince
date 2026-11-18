import { Check } from "lucide-react";
import type { ChangeEvent, ReactNode, RefObject } from "react";
import { useEffect, useId, useRef, useState } from "react";
import type { components } from "../../../api/schema";
import { VoiceKinds } from "../../../components/voicekinds";
import { Voiceprint } from "../../../components/voiceprint";
import { Button, Disclosure, Radio } from "../../../design-system/atoms";
import { useConnectAiGate } from "../../../design-system/connectai";
import { ErrorLine } from "../../../design-system/errorline";
import { Eyebrow } from "../../../design-system/eyebrow";
import { Meter } from "../../../design-system/readings";
import { formatBytes, formatNumber } from "../../../format/format";
import { useLocale, usePlural, useT } from "../../../i18n";
import { ACCEPTED_CORPUS_ATTR } from "../../../lib/voice-corpus-file";
import { VOICE_MIN_WORDS } from "../../../lib/voice-intake-core";
import { VOICE_SOURCE_MAX_BYTES } from "../../../lib/voice-refusal";
import type { ConversationQuestion } from "./conversation-machine";
import type { VoiceIngestFailure } from "./use-voice-corpus";
import { VoiceDistillPanel } from "./voice-distill";
import { type CorpusManifestEntry, manifestCount } from "./voice-manifest";
import { VoiceScene } from "./voice-scenes.result";
import { WayOnward } from "./way-onward";

// The voice act's work surface, as scenes: collect the writing, decide who
// is speaking when a transcript needs it, watch the model learn it, then
// read what it learned. One scene at a time, the same rule the company act
// follows — the rail beside them stays conversation, and every scene's own
// primary action is pinned to ITS foot, never the rail's.

export {
  VoiceBuildScene,
  VoiceResultScene,
  VoiceScene,
} from "./voice-scenes.result";

export type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];

export type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];

/**
 * The payoff, stated once, before the mechanics. The scene's own heading and
 * sub already say the CRM drafts mail in the reader's words; this band adds
 * the two things that make that credible — where the voice comes from, and
 * that it stays theirs alone — without repeating either sentence. The Core
 * sits at the size a brand line uses, not the hero size the build scene
 * reaches for, because this is context beside copy, not the scene's own
 * subject.
 */
// Why the step is worth doing, one press away. It answers a fair question, but
// it answers it for the reader who stops to ask — a permanently open band of
// rationale above the drop target competes with the drop target, which is the
// only thing on this scene anyone has to act on.
function VoiceHeroBand() {
  const t = useT();
  return (
    <Disclosure summary={t("ob.conv.voice.whyToggle")}>
      <p className="ob-voice-hero-body">{t("ob.conv.voice.heroBody")}</p>
    </Disclosure>
  );
}

// Fires the floor-reached announcement exactly once, on the transition from
// below the floor to at/above it — not on every word the server adds, which
// would talk over the reader every time a source lands. `ready` is the same
// boolean the Build button's own `disabled` reads, so the announcement can
// never fire at a moment the button disagrees with.
function useFloorReachedAnnouncement(ready: boolean, words: number): string {
  const t = useT();
  const { locale } = useLocale();
  const [message, setMessage] = useState("");
  const wasReady = useRef(ready);
  useEffect(() => {
    if (ready && !wasReady.current) {
      setMessage(
        t("ob.conv.voice.meterReady", {
          words: formatNumber(words, locale),
        }),
      );
    }
    wasReady.current = ready;
  }, [ready, words, t, locale]);
  return message;
}

/**
 * The floor meter reads the corpus total the build gate reads, never a second
 * count. The bar caps at the floor: more words still help, but the floor has
 * nothing further to fill. `ready` decides only the wording.
 */
function VoiceCorpusFloorMeter({
  words,
  ready,
}: Readonly<{ words: number; ready: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const announcement = useFloorReachedAnnouncement(ready, words);
  const shown = formatNumber(words, locale);
  const floor = formatNumber(VOICE_MIN_WORDS, locale);
  return (
    <div className="ob-voice-meter">
      <Meter
        dense
        tone="ai"
        value={Math.min(words, VOICE_MIN_WORDS)}
        max={VOICE_MIN_WORDS}
        label={t("ob.conv.voice.meterLabel", { min: floor })}
      />
      <p className="ob-voice-meter-line t-sub">
        {ready
          ? t("ob.conv.voice.meterReady", { words: shown })
          : t("ob.conv.voice.meterProgress", { words: shown, min: floor })}
      </p>
      {/* Visually hidden: the visible line above already carries this exact
          sentence once the floor clears, so this only exists to say it out
          loud the one time that happens. */}
      <p className="sr-only" role="status">
        {announcement}
      </p>
    </div>
  );
}

// The ring is the share of the build floor, and the waveform rises with it.
// The meter under it says the same count in words, so the drawing is decoration.
const PRINT_MIN_AMPLITUDE = 0.25;

// The sources the latest pick could not add, one line each, drawn beside the
// controls that took them: the board scrolls, and its foot is out of sight.
function IntakeRefusals({
  failures,
}: Readonly<{ failures: readonly VoiceIngestFailure[] }>) {
  const t = useT();
  return failures.map((failure) => (
    <ErrorLine key={failure.id}>{t(failure.i18nKey, failure.params)}</ErrorLine>
  ));
}

function VoicePrintCard({
  words,
  ready,
}: Readonly<{ words: number; ready: boolean }>) {
  const t = useT();
  const fraction = Math.min(words, VOICE_MIN_WORDS) / VOICE_MIN_WORDS;
  return (
    <section className="ob-voice-print">
      <Eyebrow>{t("ob.conv.voice.printLabel")}</Eyebrow>
      <Voiceprint
        size="medium"
        fraction={fraction}
        amplitude={PRINT_MIN_AMPLITUDE + (1 - PRINT_MIN_AMPLITUDE) * fraction}
      />
      <VoiceCorpusFloorMeter words={words} ready={ready} />
    </section>
  );
}

/**
 * The collect scene: the drop target, the sources the server has ingested,
 * and the one action that starts the build. Every number is the server's —
 * the meter counts what was actually kept, not what was handed over. Intake
 * is entirely the scene's: a file (browse or the window-wide drop) and a
 * pasted text both land here, so no other surface offers to add a source.
 * Beside it, once anything is in, the distilling panel reads the material
 * back — the reader sees their own words taken in as they add them.
 */
export function VoiceCollectScene({
  summary,
  manifest,
  failures,
  fileRef,
  onFiles,
  onAddPaste,
  onRemove,
  removing,
  onBuild,
  onSkip,
  canBuild,
  startPending,
  startError,
}: Readonly<{
  summary: CorpusSummary | null;
  manifest: readonly CorpusManifestEntry[];
  failures: readonly VoiceIngestFailure[];
  fileRef: RefObject<HTMLInputElement | null>;
  onFiles: (event: ChangeEvent<HTMLInputElement>) => void;
  onAddPaste: (text: string) => void;
  onRemove: (entry: CorpusManifestEntry) => void;
  /** The source_ref of the sample being removed, or null. */
  removing: string | null;
  onBuild: () => void;
  onSkip: () => void;
  canBuild: boolean;
  startPending: boolean;
  startError: ReactNode;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const words = summary?.total_words ?? 0;
  // The floor alone — never `canBuild`, which also folds in a request in
  // flight. A meter that dims for a busy corpus it has already cleared
  // would tell the reader they lost ground they never lost.
  const floorCleared = words >= VOICE_MIN_WORDS;
  const aiGate = useConnectAiGate("voice-collect-ai");
  // An early press names what is still missing; the foot already states the
  // floor, and saying it twice side by side reads as a glitch.
  const shortBy = VOICE_MIN_WORDS - words;
  const buildBlocker = floorCleared
    ? t("ob.conv.voice.waitForSamples")
    : plural("ob.conv.voice.floorShort", shortBy, {
        words: formatNumber(shortBy, locale),
      });
  const [pasteOpen, setPasteOpen] = useState(false);
  const [pasteText, setPasteText] = useState("");
  return (
    <VoiceScene wide>
      <VoiceHeroBand />
      <div className="ob-voice-collect">
        <div className="ob-voice-collect-main">
          <VoiceKinds />
          <div className="ob-voice-drop">
            <input
              ref={fileRef}
              type="file"
              multiple
              hidden
              accept={ACCEPTED_CORPUS_ATTR}
              onChange={onFiles}
            />
            <p className="ob-voice-drop-title">
              {t("ob.conv.voice.dropTitle")}
            </p>
            <p className="ob-voice-drop-sub t-sub">
              {t("ob.conv.voice.dropSub")}
            </p>
            <div className="ob-voice-drop-acts">
              <Button onClick={() => fileRef.current?.click()}>
                {t("ob.conv.voice.browse")}
              </Button>
              <Button variant="ghost" onClick={() => setPasteOpen(true)}>
                {t("ob.conv.voice.pasteInstead")}
              </Button>
            </div>
            <IntakeRefusals failures={failures} />
            {!pasteOpen && (
              <p className="ob-voice-drop-sub t-sub">
                {t("ob.conv.voice.dropHint", {
                  limit: formatBytes(VOICE_SOURCE_MAX_BYTES, locale),
                })}
              </p>
            )}
            {pasteOpen && (
              <div className="ob-voice-paste">
                <textarea
                  className="ob-voice-paste-area"
                  rows={5}
                  value={pasteText}
                  placeholder={t("ob.conv.voice.composer")}
                  aria-label={t("ob.conv.voice.composer")}
                  onChange={(event) => setPasteText(event.target.value)}
                />
                <div className="ob-voice-drop-acts">
                  <Button
                    variant="primary"
                    disabled={pasteText.trim() === ""}
                    onClick={() => {
                      onAddPaste(pasteText.trim());
                      setPasteText("");
                      setPasteOpen(false);
                    }}
                  >
                    {t("ob.conv.voice.pasteAdd")}
                  </Button>
                  <Button
                    variant="ghost"
                    onClick={() => {
                      setPasteText("");
                      setPasteOpen(false);
                    }}
                  >
                    {t("ob.conv.voice.pasteDiscard")}
                  </Button>
                </div>
              </div>
            )}
          </div>

          {manifest.length > 0 && (
            <section className="ob-voice-sources">
              <p className="ob-voice-sources-head t-eyebrow">
                <span>{t("ob.conv.voice.sourcesTitle")}</span>
              </p>
              <ul>
                {manifest.map((entry) => (
                  <li key={entry.ref}>
                    <span className="ob-voice-source-body">
                      <b>{entry.label}</b>
                      <small className="t-caption">
                        {manifestCount(entry, plural, locale)}
                      </small>
                    </span>
                    <Check className="ob-voice-source-check" aria-hidden />
                    <Button
                      aria-label={t("ob.conv.voice.removeSourceNamed", {
                        name: entry.label,
                      })}
                      pending={removing === entry.ref}
                      disabled={removing !== null}
                      onClick={() => onRemove(entry)}
                    >
                      {t("ob.conv.voice.removeSource")}
                    </Button>
                  </li>
                ))}
              </ul>
            </section>
          )}

          {aiGate.notice}
          <ErrorLine>{startError}</ErrorLine>
        </div>
        <div className="ob-voice-collect-side">
          <VoicePrintCard words={words} ready={floorCleared} />
          <VoiceDistillPanel manifest={manifest} summary={summary} />
        </div>
      </div>

      <div className="ob-scene-foot">
        <p>
          {canBuild
            ? t("ob.conv.voice.footReady")
            : t("ob.conv.voice.footFloor", {
                min: formatNumber(VOICE_MIN_WORDS, locale),
              })}
        </p>
      </div>
      <WayOnward
        label={t("ob.conv.voice.buildChip")}
        pending={startPending}
        blockers={[
          ...(canBuild ? [] : [buildBlocker]),
          ...(aiGate.reasonId === undefined ? [] : [t("connectAi.first")]),
        ]}
        stillNeeded={(why) => why.join(" ")}
        onGo={onBuild}
      >
        <Button variant="ghost" onClick={onSkip}>
          {t("ob.conv.voice.skipped")}
        </Button>
      </WayOnward>
    </VoiceScene>
  );
}

/**
 * The speaker decision, as the scene: which voice in a multi-speaker
 * transcript is the reader's own. This is a decision with options, so it
 * takes the whole surface the same way the collect and build scenes do —
 * never a card competing for room in the rail beside it. Every number on a
 * card (words, turns) is the preview's own count, the same one the collect
 * scene's sources list uses elsewhere.
 */
export function VoiceSpeakerScene({
  question,
  failures,
  onAnswer,
  onSkip,
}: Readonly<{
  question: ConversationQuestion;
  /** Other sources of the same pick that could not be added. */
  failures: readonly VoiceIngestFailure[];
  onAnswer: (questionId: string, value: string) => void;
  /** Leaves this file out; offered when the question carries its label. */
  onSkip: () => void;
}>) {
  const t = useT();
  const group = useId();
  const [picked, setPicked] = useState("");
  return (
    <VoiceScene>
      <div role="radiogroup" aria-label={t(question.i18nKey, question.params)}>
        <div className="ob-voice-speakers">
          {question.options.map((option) => {
            const label = option.labelKey
              ? t(option.labelKey, option.params)
              : option.label;
            const detail = option.detailKey
              ? t(option.detailKey, option.params)
              : undefined;
            const checked = picked === option.value;
            return (
              <Radio
                key={option.value}
                className={`ob-voice-speaker${checked ? " is-picked" : ""}`}
                name={group}
                value={option.value}
                checked={checked}
                onChange={() => setPicked(option.value)}
                label={
                  <span className="ob-voice-speaker-body">
                    <b>{label}</b>
                    {detail !== undefined && (
                      <small className="t-caption">{detail}</small>
                    )}
                  </span>
                }
              />
            );
          })}
        </div>
      </div>
      <IntakeRefusals failures={failures} />
      <div className="ob-scene-foot">
        <p>{t("ob.conv.voice.speakerFoot")}</p>
      </div>
      <WayOnward
        label={t("ob.conv.voice.speakerContinue")}
        blockers={picked === "" ? [t("ob.conv.voice.speakerPick")] : []}
        stillNeeded={(why) => why.join(" ")}
        onGo={() => {
          // The rail refuses an early press; this keeps a programmatic call
          // from answering with a choice nobody made.
          if (picked !== "") {
            onAnswer(question.id, picked);
          }
        }}
      >
        {question.dismissLabelKey !== undefined && (
          <Button variant="ghost" onClick={onSkip}>
            {t(question.dismissLabelKey)}
          </Button>
        )}
      </WayOnward>
    </VoiceScene>
  );
}
