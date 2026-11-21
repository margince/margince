import { useQueryClient } from "@tanstack/react-query";
import type { Dispatch } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { components } from "../../../api/schema";
import { formatNumber } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { problemMessageOf } from "../../../lib/problem";
import { isAcceptedCorpusFile } from "../../../lib/voice-corpus-file";
import type { IntakeOutcome } from "../../../lib/voice-intake-core";
import {
  intakeFile,
  intakePaste,
  intakeTranscript,
  pasteLabel,
  sourceRef,
} from "../../../lib/voice-intake-core";
import {
  readCorpusSummary,
  removeSource,
} from "../../../lib/voice-sample-remove";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { diffCorpus, useNarrationQueue } from "./narration";
import {
  type ProbeChange,
  patchVoiceProbe,
  refreshedProbeSummary,
} from "./restore";
import { SummaryOrder } from "./summary-order";
import { excerptLines } from "./voice-excerpts";
import { type CorpusManifestEntry, storedEntry } from "./voice-manifest";
import { refusalEntry, skipKeys } from "./voice-refusal-line";

// The voice corpus of the conversational shell as one hook: intake (files
// and pasted text), the preview probe that decides what a source honestly
// IS, the speaker question for conversational material, and ingestion at
// add-time. Every count the conversation shows is a server number — the
// preview's per-speaker words, the ingest's kept-of-total stats, and the
// corpus summary the meter renders. Client-side word counting only
// pre-gates empty files; it never reaches the thread.

type CorpusPreview = components["schemas"]["VoiceCorpusPreviewResult"];
type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];
type CorpusSource = components["schemas"]["VoiceCorpusSource"];
type Ingested = Extract<IntakeOutcome, { kind: "ingested" }>;

type SpeakerAsk = Readonly<{
  ref: string;
  label: string;
  content: string;
  preview: CorpusPreview;
}>;

/** One source that could not be added, as the board shows it: a refusal, a
 *  safe ingest-failed detail, or the fallback for an unexpected client bug. */
export type VoiceIngestFailure = Readonly<{
  id: string;
  i18nKey: MessageKey;
  params?: Record<string, string>;
}>;

type UseVoiceCorpusArgs = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  /** The restore probe's server meter, so a resumed session's word count
   * (and the build gate) is honest before the first new ingest. */
  initialSummary?: CorpusSummary | null;
  /** The restore probe's samples, so the list a reload shows matches the
   * meter. Only included rows: an excluded one is not in the count. */
  initialSources?: readonly CorpusSource[];
  /** The probe's word that the last summary read failed after a removal. */
  initialMeterStale?: boolean;
}>;

export function useVoiceCorpus({
  state,
  dispatch,
  initialSummary = null,
  initialSources = [],
  initialMeterStale = false,
}: UseVoiceCorpusArgs) {
  const { locale } = useLocale();
  const t = useT();
  const queryClient = useQueryClient();
  const [summary, setSummary] = useState<CorpusSummary | null>(initialSummary);
  const [manifest, setManifest] = useState<readonly CorpusManifestEntry[]>(() =>
    initialSources.filter((source) => source.included).map(storedEntry),
  );
  const [removing, setRemoving] = useState<string | null>(null);
  const [meterStale, setMeterStale] = useState(initialMeterStale);
  const [asks, setAsks] = useState<readonly SpeakerAsk[]>([]);
  const [probesInFlight, setProbesInFlight] = useState(0);
  const [failures, setFailures] = useState<readonly VoiceIngestFailure[]>([]);
  // One line per source the current pick could not add, so a batch with
  // three bad files names all three. A newer word on one source replaces it.
  const noteFailure = useCallback((failure: VoiceIngestFailure) => {
    setFailures((prev) => [
      ...prev.filter((existing) => existing.id !== failure.id),
      failure,
    ]);
  }, []);
  const summaryRef = useRef<CorpusSummary | null>(initialSummary);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  // Corpus growth narrates as a run-agnostic monotonic counter: the queue's
  // pacing keeps a multi-file burst readable, and the machine replaces the
  // words bubble in place per its stable id.
  const queue = useNarrationQueue({
    onEmit: (event) => {
      const { kind: _kind, ...entry } = event;
      dispatch({ type: "NARRATION", entry: { kind: "narration", ...entry } });
    },
  });

  const say = useCallback(
    (id: string, i18nKey: MessageKey, params?: Record<string, string>) => {
      dispatch({
        type: "NARRATION",
        entry: { kind: "narration", id, i18nKey, params },
      });
    },
    [dispatch],
  );

  // ingest/classifyUpload never THROW for a server refusal — that path
  // narrates through refusalEntry (problemMessage, already safe) and
  // resolves normally. Anything a .catch below actually receives is a
  // client-side bug that ran before the server had a chance to explain
  // itself, so its message is never trusted in front of the reader: it is
  // logged, and the narration says only that adding the source failed.
  const sayUnexpectedFailure = useCallback(
    (id: string, err: unknown) => {
      console.error("voice corpus ingest failed unexpectedly", err);
      say(id, "ob.conv.voice.ingestUnexpected");
      noteFailure({ id, i18nKey: "ob.conv.voice.ingestUnexpected" });
    },
    [say, noteFailure],
  );

  // A file that went nowhere is said on the board, the same notice a refusal
  // uses: with no rail thread left, the narration alone would reach nobody,
  // and a reader whose drop did nothing has to be told which file and why.
  const skip = useCallback(
    (name: string, reason: keyof typeof skipKeys) => {
      const id = `skip:${name}`;
      say(id, skipKeys[reason], { name });
      noteFailure({ id, i18nKey: skipKeys[reason], params: { name } });
    },
    [say, noteFailure],
  );

  // Each write is stamped when issued; only the newest summary drives the
  // meter and the word-growth narration, so a late one never rolls it back.
  const [order] = useState(() => new SummaryOrder());

  const patchProbe = useCallback(
    (change: ProbeChange) => patchVoiceProbe(queryClient, change),
    [queryClient],
  );

  const recordIngest = useCallback(
    (seq: number, outcome: Ingested, reactionKey: MessageKey) => {
      const { ref, source, stats, summary: next } = outcome;
      const newest = order.take(seq);
      // The probe outlives this step, so Back finds the sample even if the
      // reader left before the server answered.
      patchProbe((old) => ({
        summary: newest ? next : old.summary,
        meterStale: newest ? false : old.meterStale,
        sources: [
          ...old.sources.filter((stored) => stored.source_ref !== ref),
          source,
        ],
      }));
      if (!mounted.current) {
        return;
      }
      setManifest((prev) => [
        ...prev.filter((existing) => existing.ref !== ref),
        {
          ref,
          id: source.id,
          version: source.version,
          label: outcome.label,
          keptWords: stats.kept_words,
          inputWords: stats.input_words,
          transcript: outcome.transcript,
          lines: outcome.transcript ? [] : excerptLines(outcome.content),
        },
      ]);
      say(`react:${ref}`, reactionKey, {
        kept: formatNumber(stats.kept_words, locale),
        total: formatNumber(stats.input_words, locale),
        words: formatNumber(stats.kept_words, locale),
      });
      if (!newest) {
        return;
      }
      queue.push(diffCorpus(summaryRef.current, next, locale));
      summaryRef.current = next;
      setSummary(next);
      setMeterStale(false);
    },
    [locale, queue, say, patchProbe, order],
  );

  // One place the conversation learns what an intake attempt ended as: the
  // core decides, this translates the verdict into what the thread says.
  const applyOutcome = useCallback(
    (seq: number, outcome: IntakeOutcome, reactionKey: MessageKey): void => {
      // An added sample reaches the restore probe even after the step closed;
      // recordIngest keeps the step's own state behind its mounted check.
      if (outcome.kind === "ingested") {
        recordIngest(seq, outcome, reactionKey);
        return;
      }
      if (!mounted.current) {
        return;
      }
      if (outcome.kind === "skipped") {
        skip(outcome.label, outcome.reason);
        return;
      }
      if (outcome.kind === "refused") {
        const entry = refusalEntry(outcome, locale, t);
        dispatch({ type: "NARRATION", entry });
        noteFailure({
          id: entry.id,
          i18nKey: entry.i18nKey,
          params: entry.params,
        });
        return;
      }
      if (outcome.kind === "speaker-needed") {
        // A re-upload under the same name supersedes its pending question;
        // the ingest itself is idempotent on source_ref server-side.
        setAsks((prev) => [
          ...prev.filter((ask) => ask.ref !== outcome.ref),
          {
            ref: outcome.ref,
            label: outcome.label,
            content: outcome.content,
            preview: outcome.preview,
          },
        ]);
      }
    },
    [dispatch, locale, noteFailure, recordIngest, skip, t],
  );

  // Each intake is stamped when its INGEST is issued, and only the
  // newest-stamped summary may drive the meter. A response that settles late
  // carries the corpus as it stood when the server answered it, so applying it
  // after a newer one would roll the displayed total — and the build gate that
  // reads it — backwards. The stamp is taken inside the core's ingest call
  // rather than here at intake start, because a file that reads and previews
  // slowly has not written anything yet: ordering it by when the reader picked
  // it would let a preview delay decide which summary wins.
  const runIntake = useCallback(
    (
      start: (stamp: () => number) => Promise<IntakeOutcome>,
      reactionKey: MessageKey,
      failureId: string,
    ): void => {
      setProbesInFlight((count) => count + 1);
      let seq = 0;
      start(() => {
        seq = order.issue();
        return seq;
      })
        .then((outcome) => applyOutcome(seq, outcome, reactionKey))
        .catch((err: unknown) => {
          if (mounted.current) {
            sayUnexpectedFailure(failureId, err);
          }
        })
        .finally(() => {
          if (mounted.current) {
            setProbesInFlight((count) => count - 1);
          }
        });
    },
    [applyOutcome, sayUnexpectedFailure, order],
  );

  // One intake for all three entry paths: the attach button, a drop onto the
  // thread, and (via addPaste) the composer. A file of a type the corpus has
  // no reader for is refused by name, before anything is read.
  const addFiles = useCallback(
    (files: readonly File[]) => {
      setFailures([]);
      for (const file of files) {
        if (!isAcceptedCorpusFile(file.name)) {
          skip(file.name, "type");
          continue;
        }
        // The thread shows the file the moment it is handed over, before its
        // content has been read — so the bubble is identified by name, while
        // the SOURCE it becomes is keyed by what is in it.
        dispatch({
          type: "UPLOAD_ADDED",
          id: `onboarding:upload:${file.name}`,
          name: file.name,
        });
        // No known-refs set is passed here, and that is the honest answer
        // rather than an omission: this act's `manifest` holds only what THIS
        // session added, so it cannot say what the profile already stores. A
        // reader in onboarding is building a first corpus anyway, which by
        // definition has no older rows to collide with.
        runIntake(
          (stamp) => intakeFile(file, undefined, stamp),
          "ob.conv.voice.reactionDocument",
          `refuse:onboarding:upload:${file.name}`,
        );
      }
    },
    [dispatch, runIntake, skip],
  );

  // The key stays on the bare prefix, so re-pasting a text updates its row.
  // Only the label the list shows names the opening words.
  const addPaste = useCallback(
    (text: string, prefix: string) => {
      const ref = sourceRef("paste", prefix, text);
      const label = pasteLabel(prefix, text);
      setFailures([]);
      dispatch({ type: "UPLOAD_ADDED", id: ref, name: label });
      runIntake(
        (stamp) => intakePaste(ref, label, text, stamp),
        "ob.conv.voice.reactionDocument",
        `refuse:${ref}`,
      );
    },
    [dispatch, runIntake],
  );

  // The machine holds ONE pending question, so speaker asks queue here and
  // step forward whenever the conversation is back in vo.collecting. A
  // duplicate dispatch (StrictMode, a re-render race) is inert: the machine
  // rejects SPEAKER_NEEDED outside vo.collecting.
  const nextAsk = asks[0];
  useEffect(() => {
    if (
      nextAsk === undefined ||
      state.phase !== "vo.collecting" ||
      state.pendingQuestion !== null
    ) {
      return;
    }
    dispatch({
      type: "SPEAKER_NEEDED",
      question: {
        id: `speaker:${nextAsk.ref}`,
        i18nKey: "ob.conv.voice.speakerQuestion",
        params: { name: nextAsk.label },
        // A colleague's call, or one where the reader is not listed, has no
        // right answer among the options, so the file can be left out.
        dismissLabelKey: "ob.conv.voice.speakerSkip",
        options: nextAsk.preview.speakers.map((speaker) => ({
          value: speaker.label,
          label: speaker.label,
          detailKey: "ob.conv.voice.speakerOptionDetail" as const,
          params: {
            words: formatNumber(speaker.words, locale),
            turns: formatNumber(speaker.turns, locale),
          },
        })),
      },
    });
  }, [nextAsk, state.phase, state.pendingQuestion, dispatch, locale]);

  // A named speaker filters the ingest, so only their words reach the meter.
  // Without a value the owner skipped the file, and none of it is sent.
  const answerSpeaker = useCallback(
    (questionId: string, value: string | null) => {
      const ask = asks.find(
        (candidate) => `speaker:${candidate.ref}` === questionId,
      );
      if (!ask) {
        return;
      }
      setAsks((prev) => prev.filter((candidate) => candidate.ref !== ask.ref));
      setFailures([]);
      if (value === null) {
        return;
      }
      runIntake(
        (stamp) =>
          intakeTranscript(ask.ref, ask.label, ask.content, value, stamp),
        "ob.conv.voice.reactionTranscript",
        `refuse:${ask.ref}`,
      );
    },
    [asks, runIntake],
  );

  // The removal's summary is the newest corpus there is, so it outranks any
  // ingest issued before it, as a later ingest would.
  const applyRemovalSummary = useCallback(
    (seq: number, next: CorpusSummary) => {
      if (!order.take(seq)) {
        return;
      }
      patchProbe(() => ({ summary: next, meterStale: false }));
      if (mounted.current) {
        summaryRef.current = next;
        setSummary(next);
        setMeterStale(false);
      }
    },
    [patchProbe, order],
  );

  const removeSample = useCallback(
    (entry: CorpusManifestEntry) => {
      setFailures([]);
      setRemoving(entry.ref);
      const seq = order.issue();
      removeSource(entry.id, entry.version)
        .then(
          (profileId) => {
            // Gone at the server once the DELETE lands, whatever the meter
            // read after it answers, and whether or not the step is still open.
            order.removed(seq);
            patchProbe((old) => ({
              sources: old.sources.filter((source) => source.id !== entry.id),
            }));
            if (mounted.current) {
              setManifest((prev) =>
                prev.filter((row) => row.ref !== entry.ref),
              );
            }
            return readCorpusSummary(profileId).then(
              (next) => applyRemovalSummary(seq, next),
              // The meter still counts the removed words, so Build waits for a fresh read.
              () => {
                patchProbe(() => ({ meterStale: true }));
                if (mounted.current) setMeterStale(true);
                return refreshedProbeSummary(queryClient).then(
                  (next) => next && applyRemovalSummary(seq, next),
                );
              },
            );
          },
          (err: unknown) => {
            if (mounted.current) {
              noteFailure({
                id: `remove:${entry.ref}`,
                i18nKey: "ob.conv.voice.removeFailed",
                params: {
                  name: entry.label,
                  detail: problemMessageOf(err, t),
                },
              });
            }
          },
        )
        .finally(() => {
          if (mounted.current) {
            setRemoving(null);
          }
        });
    },
    [applyRemovalSummary, noteFailure, patchProbe, queryClient, t, order],
  );

  return {
    summary,
    manifest,
    removeSample,
    /** The source_ref of the sample being removed, or null. */
    removing,
    addFiles,
    addPaste,
    answerSpeaker,
    /** True while any probe, ingest, removal or speaker question is still
     * open: a build starting now would misrepresent what the voice is made of. */
    busy:
      probesInFlight > 0 || asks.length > 0 || removing !== null || meterStale,
    /** True while no fresh summary has answered for the last removal. */
    meterStale,
    /** Each source of the latest pick the corpus could not add; cleared
     * when the reader hands over the next file, paste or speaker answer. */
    failures,
  };
}
