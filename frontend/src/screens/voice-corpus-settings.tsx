import { useEffect, useRef, useState } from "react";
import { Button, Disclosure, Radio } from "../design-system/atoms";
import { FileDropzoneControl } from "../design-system/filedropzone";
import { SettingRow } from "../design-system/settingrow";
import { formatNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { problemMessageOf } from "./common";
import { useFileDrop } from "./use-file-drop";
import type { IntakeNotice, SpeakerAsk } from "./use-voice-intake";
import { useVoiceIntake } from "./use-voice-intake";
import { ACCEPTED_CORPUS_ATTR, VOICE_MIN_WORDS } from "./voice-intake-core";

// The Settings intake: hand over files, and answer who is speaking when one
// turns out to be a conversation. It owns the intake hook so the build control
// beside it can be disabled while a source is still arriving — a build started
// mid-ingest describes a corpus that no longer exists by the time it finishes.
//
// Files are the only way in. Onboarding also takes pasted text because it is a
// conversation and a paste is a turn in it; here a reader arrives with their
// sent mail already exported, and one control for one act is what a settings
// row can carry. What the card explains beside that control is the part
// onboarding narrates: what the voice is for, which writing teaches it, and
// why only the reader's own words count.
//
// Everything the intake has to say back — the unanswered speaker questions and
// the per-source notices — lives in this component's hook and renders on the
// card under the zone, where a reader who has just handed over six files can
// see what happened to each.

type VoiceCorpusIntakeProps = Readonly<{
  /** null for an owner who has no profile yet: the first sample mints it. */
  profileId: string | null;
  onChanged: () => void;
  /** Told when intake is in progress, so the caller can hold the build. */
  onBusyChange?: (busy: boolean) => void;
  /** The first sample is the one that MINTS the profile: the row names itself
   * for that, and there is no meter yet to say how far the floor is, so the
   * card states the floor itself. */
  first?: boolean;
}>;

export function VoiceCorpusIntake({
  profileId,
  onChanged,
  onBusyChange,
  first = false,
}: VoiceCorpusIntakeProps) {
  const t = useT();
  const { locale } = useLocale();
  const intake = useVoiceIntake({ profileId, onChanged });
  const zoneRef = useRef<HTMLDivElement>(null);

  // The zone's own input takes every drop that lands on it. This hook is never
  // armed: it only keeps a file that misses the zone from navigating the
  // browser away from the app, which is what a stray drop does by default.
  useFileDrop({ container: zoneRef, active: false, onFiles: intake.addFiles });

  // Told AFTER the render that changed it: calling a parent's setState from a
  // render body updates one component while another is rendering, which React
  // does not support.
  const busy = intake.busy;
  useEffect(() => {
    onBusyChange?.(busy);
  }, [busy, onBusyChange]);

  return (
    <div ref={zoneRef}>
      <SettingRow
        label={t(
          first ? "settings.voice.addFirstLabel" : "settings.voice.addSource",
        )}
        description={t("settings.voice.dropHint")}
        layout="stack"
        control={(control) => (
          <div className="vdna-zone settingrow-measure">
            <FileDropzoneControl
              control={control}
              emptyLabel={t("settings.voice.dropEmpty")}
              multiple
              accept={ACCEPTED_CORPUS_ATTR}
              onPick={(file) => intake.addFiles([file])}
            />
            {intake.pendingAsk && (
              // Keyed by the source: when the queue advances to the next file
              // the panel is a NEW panel, so the previous file's chosen
              // speaker cannot survive into a question about different
              // people.
              <SpeakerPanel
                key={intake.pendingAsk.ref}
                ask={intake.pendingAsk}
                onAnswer={intake.answerSpeaker}
                onDismiss={intake.dismissAsk}
              />
            )}
            {/* Polite, so the outcome of a drop reaches a screen reader as it
                lands: the zone's own live region only ever states the
                invitation, because the input is cleared after every pick. A
                refusal is still an alert of its own. */}
            {intake.notices.length > 0 && (
              <ul className="vdna-notices" aria-live="polite">
                {intake.notices.map((notice) => (
                  <NoticeRow key={notice.ref} notice={notice} />
                ))}
              </ul>
            )}
            <WhatTeachesTheVoice />
            {first && (
              <p className="t-small vdna-floornote">
                {t("settings.voice.floorNote", {
                  min: formatNumber(VOICE_MIN_WORDS, locale),
                })}
              </p>
            )}
            <Disclosure summary={t("settings.voice.whyToggle")}>
              <p className="t-small">{t("settings.voice.whyBody")}</p>
            </Disclosure>
          </div>
        )}
      />
    </div>
  );
}

// Which writing teaches the voice and which teaches it somebody else's. The
// reader sees this beside the zone every time, not once in onboarding: the
// question "what should I upload?" is asked at the moment of uploading.
function WhatTeachesTheVoice() {
  const t = useT();
  return (
    <div className="vdna-works">
      <p className="t-small vdna-label">{t("settings.voice.worksTitle")}</p>
      <ul className="t-small vdna-works-list">
        <li>{t("settings.voice.worksEmails")}</li>
        <li>{t("settings.voice.worksDocs")}</li>
        <li>{t("settings.voice.worksTranscripts")}</li>
      </ul>
      <p className="t-small vdna-works-not">{t("settings.voice.worksNot")}</p>
    </div>
  );
}

// A file the preview found several speakers in: only the owner's own turns may
// become their voice, so the source waits here until they say which is theirs.
function SpeakerPanel({
  ask,
  onAnswer,
  onDismiss,
}: Readonly<{
  ask: SpeakerAsk;
  onAnswer: (speakerLabel: string) => void;
  onDismiss: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [choice, setChoice] = useState<string | null>(null);
  return (
    <fieldset className="vdna-speaker">
      <legend className="vdna-label">
        {t("settings.voice.speakerQuestion", { name: ask.label })}
      </legend>
      <p className="t-small">{t("settings.voice.speakerWhy")}</p>
      <ul className="vdna-speaker-options">
        {ask.preview.speakers.map((speaker) => (
          <li key={speaker.label}>
            <Radio
              name={`speaker:${ask.ref}`}
              checked={choice === speaker.label}
              onChange={() => setChoice(speaker.label)}
              label={`${speaker.label} · ${t("settings.voice.speakerDetail", {
                words: formatNumber(speaker.words, locale),
                turns: formatNumber(speaker.turns, locale),
              })}`}
            />
          </li>
        ))}
      </ul>
      <div className="vdna-composer-actions">
        <Button
          small
          variant="primary"
          disabled={choice === null}
          onClick={() => choice !== null && onAnswer(choice)}
        >
          {t("settings.voice.speakerConfirm")}
        </Button>
        <Button small onClick={onDismiss}>
          {t("settings.voice.speakerDismiss")}
        </Button>
      </div>
    </fieldset>
  );
}

// What one finished intake says to the reader. A refusal the core did not
// recognize quotes the server's own detail rather than inventing a reason.
function noticeText(
  t: ReturnType<typeof useT>,
  notice: IntakeNotice,
  locale: Locale,
): string {
  switch (notice.kind) {
    case "kept":
      // Kept-of-total is the story of a speaker filter. A document keeps
      // every word, and "kept 531 of 531" reads as if something was judged.
      return notice.transcript
        ? t("settings.voice.noticeKept", {
            name: notice.label,
            kept: formatNumber(notice.keptWords ?? 0, locale),
            total: formatNumber(notice.inputWords ?? 0, locale),
          })
        : t("settings.voice.noticeAdded", {
            name: notice.label,
            words: formatNumber(notice.keptWords ?? 0, locale),
          });
    case "skippedType":
      return t("settings.voice.noticeSkippedType", { name: notice.label });
    case "skippedEmpty":
      return t("settings.voice.noticeSkippedEmpty", { name: notice.label });
    case "dismissed":
      return t("settings.voice.noticeDismissed", { name: notice.label });
    case "askQueueFull":
      return t("settings.voice.noticeAskQueueFull", { name: notice.label });
    case "refused":
      switch (notice.reason) {
        case "unattributed":
          return t("settings.voice.refusalUnattributed", {
            name: notice.label,
          });
        case "speaker":
          return t("settings.voice.refusalSpeaker", { name: notice.label });
        case "unsupported":
          return t("settings.voice.refusalUnsupported", { name: notice.label });
        default:
          return t("settings.voice.noticeFailed", {
            name: notice.label,
            detail: problemMessageOf(notice.problem, t),
          });
      }
    case "failed":
      return t("settings.voice.noticeUnexpected", { name: notice.label });
  }
}

function NoticeRow({ notice }: Readonly<{ notice: IntakeNotice }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <li
      className={`t-small vdna-notice vdna-notice-${notice.tone}`}
      role={notice.tone === "warn" ? "alert" : undefined}
    >
      {noticeText(t, notice, locale)}
    </li>
  );
}
