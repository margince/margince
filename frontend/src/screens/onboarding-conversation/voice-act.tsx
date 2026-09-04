import type { ChangeEvent, Dispatch, RefObject } from "react";
import { useRef } from "react";
import type { components } from "../../api/schema";
import { ordinalNumber } from "../../format/format";
import { useT } from "../../i18n";
import { problemMessageOf } from "../common";
import { useFileDrop } from "../use-file-drop";
import { parseVoiceInsights } from "../voice-insights";
import { VOICE_MIN_WORDS } from "../voice-intake-core";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import { railStops } from "./rail";
import { useVoiceBuild } from "./use-voice-build";
import { useVoiceCorpus } from "./use-voice-corpus";
import type { VoiceContinueReason } from "./voice-artifact";
import { VoiceActArtifact } from "./voice-artifact";
import {
  VoiceBuildScene,
  VoiceCollectScene,
  VoiceResultScene,
  VoiceSpeakerScene,
} from "./voice-scenes";
import { ConversationWorkbench, useConfiguredModel } from "./workbench";

// The voice act driver: intake and ingestion live in useVoiceCorpus, the
// build lifecycle in useVoiceBuild. Every source — a browsed file, a window
// drop, a pasted text — lands through the collect scene, which is the
// board's own content; the room asks one question at a time, so there is no
// separate rail transcript to keep in step with it.

type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];
type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];

type VoiceActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  /** The restore probe's server meter for a resumed session; null fresh. */
  initialSummary?: CorpusSummary | null;
}>;

export function VoiceAct({ state, dispatch, initialSummary }: VoiceActProps) {
  const t = useT();
  const machine = useRef(state);
  machine.current = state;
  const corpus = useVoiceCorpus({ state, dispatch, initialSummary });
  const build = useVoiceBuild({ dispatch, machine });
  const fileRef = useRef<HTMLInputElement>(null);

  const collecting =
    state.phase === "vo.collecting" || state.phase === "vo.speaker";
  const serverWords = corpus.summary?.total_words ?? 0;
  const canBuild =
    state.phase === "vo.collecting" &&
    serverWords >= VOICE_MIN_WORDS &&
    !corpus.busy &&
    !build.start.isPending;

  const onFiles = (event: ChangeEvent<HTMLInputElement>) => {
    corpus.addFiles(Array.from(event.target.files ?? []));
    event.target.value = "";
  };

  // The scene promises "drop files anywhere in this conversation", so the
  // WHOLE window is the drop target (container null) — a file landing on the
  // rail, the artifact panel, or a layout gap must feed the corpus. Outside
  // the collecting phases a stray drop is still neutralized: the browser's
  // default is to NAVIGATE to the dropped file, which would tear the user out
  // of the onboarding mid-act.
  const { dragOver } = useFileDrop({
    container: null,
    active: collecting,
    onFiles: corpus.addFiles,
  });

  const handleAnswer = (questionId: string, value: string) => {
    dispatch({ type: "QUESTION_ANSWERED", questionId, value });
    corpus.answerSpeaker(questionId, value);
  };

  const presence = presenceFor(state);
  const configuredModel = useConfiguredModel();

  // Where the journey stands, in the rail's own counting.
  const stops = railStops(state.memberPath);
  const eyebrow = t("ob.conv.scene.step", {
    n: ordinalNumber(stops.findIndex((stop) => stop.key === "voice") + 1),
    m: ordinalNumber(stops.length),
    label: t("ob.rail.voice"),
  });

  const scene = (
    <VoiceSurface
      state={state}
      dispatch={dispatch}
      corpus={corpus}
      build={build}
      canBuild={canBuild}
      fileRef={fileRef}
      onFiles={onFiles}
      onAnswer={handleAnswer}
      model={configuredModel}
    />
  );

  return (
    <ConversationWorkbench
      core={presence.core}
      progress={presence.progress}
      // The build scene draws the Core itself, with the progress ring inside
      // it; the room's own would be a second orb saying the same thing.
      coreHidden={state.phase === "vo.building"}
      railState={state}
      status={t(
        state.phase === "vo.building"
          ? "ob.conv.voice.statusBuilding"
          : "ob.ai.ready",
      )}
      {...boardHeading(state, eyebrow, build.builtVersion.data ?? null, t)}
    >
      {/* The whole window stays the drop target while collecting (see
          useFileDrop below); the board is what shows that now, since there
          is no rail thread left to ring. */}
      <div className={dragOver ? "ob-conv-dragover" : undefined}>{scene}</div>
      {corpus.failure && (
        <p className="ob-conv-notice" role="alert">
          {t(corpus.failure.i18nKey, corpus.failure.params)}
        </p>
      )}
    </ConversationWorkbench>
  );
}

/**
 * What the room says this screen is, per phase. THE QUESTION IS THE TITLE
 * while the speaker decision is pending, the same rule the company act's
 * clarify branch follows. Every other phase points at the scene's own
 * existing headline; `sceneTitle`/`sceneSub` describe the whole voice step,
 * so they also stand in for the fallback branch (skipped, failed, deferred)
 * that no longer has a scene heading of its own.
 */
function boardHeading(
  state: ConversationState,
  eyebrow: string,
  builtVersion: VoiceProfileVersion | null,
  t: ReturnType<typeof useT>,
): Readonly<{ eyebrow?: string; title: string; sub?: string }> {
  if (state.phase === "vo.speaker" && state.pendingQuestion !== null) {
    return {
      eyebrow,
      title: t(state.pendingQuestion.i18nKey, state.pendingQuestion.params),
    };
  }
  if (state.phase === "vo.building") {
    return { eyebrow, title: t("ob.conv.voice.buildingTitle") };
  }
  if (state.phase === "vo.result" && state.lastBuildStatus === "succeeded") {
    const data =
      builtVersion !== null ? parseVoiceInsights(builtVersion) : null;
    // A version with no reserved held-out samples (the starter-corpus case:
    // too few sources to spare any) never carries a sample draft, and the
    // sub line says so rather than promising one.
    const hasSample = data !== null && data.sampleDrafts.length > 0;
    return {
      eyebrow,
      title: t("ob.conv.voice.resultTitle"),
      sub: t(
        data !== null && !hasSample
          ? "ob.conv.voice.resultSubNoSample"
          : "ob.conv.voice.resultSub",
      ),
    };
  }
  // A build that did not finish, or is waiting on budget, is the room's own
  // headline: left under "teach me how you write" the reader takes the dossier
  // below for the result and never learns nothing was built.
  if (state.phase === "vo.result" && state.lastBuildStatus === "failed") {
    return { eyebrow, title: t("ob.conv.build.failed") };
  }
  if (state.phase === "vo.result" && state.lastBuildStatus === "deferred") {
    return { eyebrow, title: t("ob.conv.build.deferred") };
  }
  return {
    eyebrow,
    title: t("ob.conv.voice.sceneTitle"),
    sub: t("ob.conv.voice.sceneSub"),
  };
}

// The fallback branch's own Continue: `vo.skipped` has nothing left to
// finish, a failed build offers the retry the machine still permits, a
// deferred one has already said so honestly in its rail outcome and only
// waits for the human to move on. Every case shares the one action.
function continueBarFor(
  state: ConversationState,
  build: ReturnType<typeof useVoiceBuild>,
  dispatch: Dispatch<ConversationEvent>,
): Readonly<{
  reason: VoiceContinueReason;
  onContinue: () => void;
  retryPending?: boolean;
  onRetry?: () => void;
}> | null {
  const onContinue = () => dispatch({ type: "VOICE_DONE" });
  if (state.phase === "vo.skipped") {
    return { reason: "skipped", onContinue };
  }
  if (state.phase === "vo.result" && state.lastBuildStatus === "failed") {
    return {
      reason: "failed",
      onContinue,
      retryPending: build.start.isPending,
      onRetry: () => build.start.mutate(),
    };
  }
  if (state.phase === "vo.result" && state.lastBuildStatus === "deferred") {
    return { reason: "deferred", onContinue };
  }
  return null;
}

/**
 * Which scene the voice act's work surface is showing, and nothing else:
 * collect the writing, decide who is speaking, watch the model learn it,
 * then read what it learned. Outside those the corpus dossier stands in,
 * carrying Continue itself once there is nothing left a scene can own.
 * Extracted from the act driver so the driver stays about events, not
 * about staging.
 */
function VoiceSurface({
  state,
  dispatch,
  corpus,
  build,
  canBuild,
  fileRef,
  onFiles,
  onAnswer,
  model,
}: Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  corpus: ReturnType<typeof useVoiceCorpus>;
  build: ReturnType<typeof useVoiceBuild>;
  canBuild: boolean;
  fileRef: RefObject<HTMLInputElement | null>;
  onFiles: (event: ChangeEvent<HTMLInputElement>) => void;
  onAnswer: (questionId: string, value: string) => void;
  model: string;
}>) {
  const t = useT();
  if (state.phase === "vo.collecting") {
    return (
      <VoiceCollectScene
        summary={corpus.summary}
        manifest={corpus.manifest}
        fileRef={fileRef}
        onFiles={onFiles}
        onAddPaste={(text) =>
          corpus.addPaste(text, t("ob.conv.voice.pasteSource"))
        }
        onBuild={() => build.start.mutate()}
        onSkip={() => dispatch({ type: "VOICE_SKIPPED" })}
        canBuild={canBuild}
        startPending={build.start.isPending}
        startError={
          build.start.isError ? problemMessageOf(build.start.error, t) : null
        }
      />
    );
  }
  if (state.phase === "vo.speaker" && state.pendingQuestion !== null) {
    return (
      <VoiceSpeakerScene question={state.pendingQuestion} onAnswer={onAnswer} />
    );
  }
  if (state.phase === "vo.building") {
    return (
      <VoiceBuildScene
        stage={build.stage}
        summary={corpus.summary}
        sources={corpus.manifest.length}
        model={model}
      />
    );
  }
  if (state.phase === "vo.result" && state.lastBuildStatus === "succeeded") {
    return (
      <VoiceResultScene
        loading={build.builtVersion.isPending}
        version={build.builtVersion.data ?? null}
        onContinue={() => dispatch({ type: "VOICE_DONE" })}
        onRevise={() => dispatch({ type: "VOICE_REVISE" })}
      />
    );
  }
  // Everything a scene does not own — a failed or deferred build, the skip
  // — keeps the corpus dossier, now carrying its own pinned Continue. The
  // build scene above has already claimed every building phase, so no
  // tracker runs here.
  return (
    <VoiceActArtifact
      summary={corpus.summary}
      manifest={corpus.manifest}
      stage={build.stage}
      building={false}
      continueBar={continueBarFor(state, build, dispatch) ?? undefined}
    />
  );
}
