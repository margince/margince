// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import { useSettledValue } from "../../design-system/debouncedsearch";
import { postMultipart } from "../../lib/multipart";
import { fromHere } from "../../lib/navigatefrom";
import { problemStatus, unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import {
  addressedRunNow,
  useAddressedRun,
  usePresetContact,
} from "./importaddress";
import {
  choicesOf,
  type DraftPatch,
  type Edits,
  editsPatch,
  fullPatch,
  holdRun,
  NO_EDITS,
  useDraftWrites,
} from "./importdraft";
import {
  defaultSourceName,
  filesOf,
  sourceNameFits,
  stepOfRun,
  uploadObjectFor,
} from "./importflow";
import { asProfile } from "./importprofile";
import {
  IMPORTS_KEY,
  NO_SUCH_RUN,
  orInFlight,
  useImportReport,
  useImportRun,
  useLandedRecords,
  useStopImportTest,
  useUndoImport,
} from "./importrun";
import type {
  ImportFormat,
  ImportObject,
  ImportProfile,
  ImportRun,
} from "./importtypes";

type UploadInput = Readonly<{
  file: File;
  object: ImportObject | undefined;
  /** The runs already made, read for the source name a ZIP starts with. */
  recent: readonly ImportRun[] | undefined;
  /** A type change of a single CSV, which keeps the name the reader set. */
  retype: boolean;
  /** The draft this upload takes the place of, cancelled once it lands. */
  replaces: string | null;
}>;

/** The upload of a file the reader just picked, typed by its name or preset. */
export function firstUpload(
  file: File,
  format: ImportFormat,
  recent: readonly ImportRun[] | undefined,
  preset?: ImportObject,
): UploadInput {
  return {
    file,
    object: uploadObjectFor(file.name, format, preset),
    recent,
    retype: false,
    replaces: null,
  };
}

/** How the server typed a CSV sent with no type: by its header, or not. */
export type TypeGuess = NonNullable<ImportProfile["object_chosen_by"]>;

async function postSource(input: UploadInput): Promise<ImportProfile> {
  const payload = await postMultipart("/v1/imports/sources", {
    ...(input.object ? { object: input.object } : {}),
    file: input.file,
  });
  return asProfile(payload);
}

// The draft an upload replaces is cancelled once the upload lands, and only
// then: a refused upload leaves the reader on the draft they had.
async function uploadReplacing(
  input: UploadInput,
  retire: (id: string) => void,
): Promise<ImportProfile> {
  const profile = await postSource(input);
  if (input.replaces) retire(input.replaces);
  return profile;
}

// What a landed upload starts the reader's edits from: none, so nothing is
// saved until they change something. A type change keeps the name and tag.
function editsAfterUpload(retype: boolean, current: Edits): Edits {
  return retype
    ? {
        ...NO_EDITS,
        sourceName: current.sourceName,
        contextTagID: current.contextTagID,
      }
    : NO_EDITS;
}

// A run read that names no run: an id no run has, or no id at all.
function namesNoRun(
  run: Readonly<{ isError: boolean; error: unknown }>,
): boolean {
  return run.isError && NO_SUCH_RUN.includes(problemStatus(run.error));
}

// How the server typed the run's CSV, while the run still holds that type. A
// type the reader saved on the run, or a draft they took to the map, wins.
function guessOf(
  profile: ImportProfile | null,
  run: ImportRun | undefined,
): TypeGuess | null {
  if (!profile?.object_chosen_by || profile.parts) return null;
  const held = run?.object ?? profile.object;
  const kept = held === profile.object && run?.draft_step !== "map";
  return kept ? profile.object_chosen_by : null;
}

// A CSV the server could not type shows no type until the reader picks one.
function withUntyped(edits: Edits, file: string | undefined): Edits {
  if (file === undefined || file in edits.objects) return edits;
  return { ...edits, objects: { ...edits.objects, [file]: null } };
}

// A source name the server refuses stays on screen with its reason and is not
// sent, so the edits saved leave it out.
function savableEdits(edits: Edits): Edits {
  return sourceNameFits(edits.sourceName ?? "")
    ? edits
    : { ...edits, sourceName: undefined };
}

type Test = Readonly<{ id: string; patch: DraftPatch }>;

// The run flow: upload, check the files, map their columns, test, import and
// read the result. The run is the state. The step, the profile and the choices
// are read from it, so a reload or a deep link lands where the reader was.
// Only the upload in flight and the edits not yet answered live here.
export function useImportWizard() {
  const queryClient = useQueryClient();
  const generation = useRef(0);
  const [source, setSource] = useState<File | null>(null);
  const [edits, setEditsState] = useState<Edits>(NO_EDITS);
  const latest = useRef(edits);
  const [runId, setRunId] = useAddressedRun();
  const run = useImportRun(runId);
  // The contact a file of activities is filed under: the address names it for
  // a new upload, and the run keeps it after that.
  const presetContact =
    usePresetContact() ?? run.data?.preset_links?.[0]?.id ?? null;
  const status = run.data?.status;
  const report = useImportReport(runId, status, run.data?.version);
  const undo = useUndoImport();
  const cancelRun = useStopImportTest();
  const retire = useStopImportTest();
  // The edits last sent, or folded into the test run's save.
  const sent = useRef(NO_EDITS);
  // A version conflict's edits lose to the other tab: the screen shows what
  // the run holds and nothing is re-sent over it. A save refused because the
  // run was no longer a draft goes again once the run, read again, is one.
  const writes = useDraftWrites(
    () => {
      latest.current = NO_EDITS;
      setEditsState(NO_EDITS);
    },
    () => {
      sent.current = NO_EDITS;
    },
  );
  useLandedRecords([status]);

  // An edit reads the latest edits, not the render's, so two edits in one
  // tick both land.
  const edit = (change: (current: Edits) => Edits) => {
    latest.current = change(latest.current);
    setEditsState(latest.current);
  };

  const upload = useMutation({
    mutationFn: async (input: UploadInput) => {
      const at = generation.current;
      const profile = await uploadReplacing(input, retire.mutate);
      return {
        at,
        input,
        profile,
        name: defaultSourceName(profile, input.recent),
      };
    },
    onSuccess: ({ at, input, profile }) => {
      if (at !== generation.current) return;
      setSource(input.file);
      edit((current) => editsAfterUpload(input.retype, current));
      setRunId(profile.run_id);
    },
  });

  const profile = run.data?.profile ?? upload.data?.profile ?? null;
  const zip = profile?.parts !== undefined;
  const files = profile ? filesOf(profile) : [];
  const proposedName =
    upload.data?.name ?? profile?.suggested_source_name ?? "";
  const guess = guessOf(profile, run.data);
  const untyped = guess === "none" ? files[0]?.file : undefined;
  const choices = choicesOf(
    files,
    zip,
    run.data,
    withUntyped(edits, untyped),
    proposedName,
  );

  const test = useMutation({
    mutationFn: async ({ id, patch }: Test) => {
      const saved = await writes.patch(id, patch);
      return unwrap(
        await api.POST("/imports/{id}/validate", {
          params: { path: { id } },
          body: { version: saved.version },
        }),
      );
    },
    // The run the test answered is held until it is read again, so the draft
    // stays closed from the moment the test is asked for.
    onSuccess: (validated) => holdRun(queryClient, validated),
    onSettled: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });

  // A draft takes saves only while no test or cancel of it is on its way. A
  // save landing after either is refused: the run is no longer a draft.
  const draftOpen =
    profile !== null &&
    (status === undefined || status === "pending") &&
    !test.isPending &&
    !cancelRun.isPending;

  // Edits settle for a moment, then go to the server as one save. Only the
  // newest edits are sent: an older settled value was sent, or folded into the
  // test run's save, already.
  const settled = useSettledValue(edits);
  const savable = savableEdits(settled);
  const toSave = editsPatch(
    files,
    zip,
    choicesOf(files, zip, run.data, savable, proposedName),
    savable,
  );
  const { save } = writes;
  useEffect(() => {
    const stale = settled !== edits || settled === sent.current;
    if (stale || runId === null || !draftOpen) return;
    sent.current = settled;
    if (toSave) save(runId, toSave);
  }, [settled, edits, runId, draftOpen, toSave, save]);

  // The version is the one the reader's report was read at, so a run that
  // changed since is refused rather than imported unread.
  const approve = useMutation({
    mutationFn: async ({ id, version }: Pick<ImportRun, "id" | "version">) =>
      orInFlight(async () =>
        unwrap(
          await api.POST("/imports/{id}/approve", {
            params: { path: { id } },
            body: { version },
          }),
        ),
      ),
    onSettled: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });

  const effective = status ?? (profile ? "pending" : undefined);
  const draftStep = edits.step ?? run.data?.draft_step ?? "files";
  const step = runId === null ? "upload" : stepOfRun(effective, draftStep);
  const anyway = test.isPending
    ? test.variables?.patch.on_broken_link === "drop_link"
    : run.data?.on_broken_link === "drop_link";

  const reset = () => {
    generation.current += 1;
    upload.reset();
    retire.reset();
    test.reset();
    approve.reset();
    undo.reset();
    writes.clearConflict();
    latest.current = NO_EDITS;
    sent.current = NO_EDITS;
    setEditsState(NO_EDITS);
    setSource(null);
    setRunId(null);
  };

  return {
    step,
    profile,
    files,
    objects: choices.objects,
    /** The run address names no run the reader can open. */
    missing: namesNoRun(run),
    /** How this upload's type came from its columns, if they chose it. */
    typeGuess: guess,
    // A ZIP part's type changes in place, and its mapping starts again from
    // that type's proposal. A single CSV is profiled for one type, so another
    // type uploads it again. The type it was read as is a save, which tells
    // the server the reader chose it.
    setFileObject: (file: string, object: ImportObject | null) => {
      if (profile && !zip && source && object && object !== profile.object) {
        upload.mutate({
          file: source,
          object,
          recent: undefined,
          retype: true,
          replaces: runId,
        });
        return;
      }
      const suggested = files.find((one) => one.file === file)?.suggested;
      edit((current) => ({
        ...current,
        objects: { ...current.objects, [file]: object },
        mapping: {
          ...current.mapping,
          [file]: object ? (suggested?.[object] ?? {}) : {},
        },
      }));
    },
    mapping: choices.mapping,
    setColumnTarget: (file: string, column: string, target: string) =>
      edit((current) => ({
        ...current,
        mapping: {
          ...current.mapping,
          [file]: {
            ...(current.mapping[file] ?? choices.mapping[file]),
            [column]: target,
          },
        },
      })),
    sourceName: choices.sourceName,
    setSourceName: (sourceName: string) =>
      edit((current) => ({ ...current, sourceName })),
    contextTagID: choices.contextTagID,
    setContextTagID: (contextTagID: string) =>
      edit((current) => ({ ...current, contextTagID })),
    runId,
    /** The contact every activity of this run is filed under, if one is. */
    presetContact,
    run,
    report,
    upload,
    test,
    // Saves the whole draft, then tests it: the test reads what is on screen.
    runTest: () => {
      if (runId) {
        sent.current = latest.current;
        test.mutate({
          id: runId,
          patch: fullPatch(files, zip, choices, run.data, presetContact),
        });
      }
    },
    saveFailure: writes.failure,
    conflict: writes.conflict,
    dismissConflict: writes.clearConflict,
    approve,
    undo,
    // The files and the map open again while the run is a draft whose file
    // the server still keeps.
    editable: runId === null || (effective === "pending" && profile !== null),
    anyway,
    // Rows with broken links come in without the link, or stay out. The
    // choice tests the run again, so the report approved is the one shown.
    setAnyway: (next: boolean) => {
      if (runId) {
        test.mutate({
          id: runId,
          patch: { on_broken_link: next ? "drop_link" : "skip_row" },
        });
      }
    },
    goTo: (next: "files" | "map") =>
      edit((current) => ({ ...current, step: next })),
    // Closes the draft on the server and returns to the hub, where it shows
    // as canceled.
    cancel: () => {
      if (runId === null) return;
      const from = fromHere();
      // The reader is discarding the draft, so an edit still settling is not sent.
      sent.current = latest.current;
      cancelRun.mutate(runId, {
        // Cancelling a draft another tab is testing only stops that test and
        // leaves the draft, so the page stays on it until it is cancelled.
        onSuccess: (stopped) => {
          // A reader who moved on keeps the page, and the run they now hold.
          const holdsIt = from.stillHere() && addressedRunNow() === runId;
          if (stopped?.status !== "cancelled" || !holdsIt) return;
          reset();
          navigate(settingsHref("import"));
        },
      });
    },
    cancelling: cancelRun,
    /** The cancel of a draft an upload replaced; its refusal is shown. */
    retiring: retire,
    reset,
  };
}

export type ImportWizardState = ReturnType<typeof useImportWizard>;
