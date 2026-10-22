// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import { useSettledValue } from "../../design-system/debouncedsearch";
import { postMultipart } from "../../lib/multipart";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { useAddressedRun, usePresetContact } from "./importaddress";
import {
  choicesOf,
  type DraftPatch,
  type Edits,
  editsPatch,
  fullPatch,
  NO_EDITS,
  useDraftWrites,
} from "./importdraft";
import { defaultSourceName, filesOf, stepOfRun } from "./importflow";
import { asProfile } from "./importprofile";
import {
  IMPORTS_KEY,
  orInFlight,
  useImportReport,
  useImportRun,
  useLandedRecords,
  useStopImportTest,
  useUndoImport,
} from "./importrun";
import type { ImportObject, ImportProfile, ImportRun } from "./importtypes";

type UploadInput = Readonly<{
  file: File;
  object: ImportObject | undefined;
  /** The runs already made, read for the source name a ZIP starts with. */
  recent: readonly ImportRun[] | undefined;
  /** A type change of a single CSV, which keeps the name the reader set. */
  retype: boolean;
}>;

async function postSource(input: UploadInput): Promise<ImportProfile> {
  const payload = await postMultipart("/v1/imports/sources", {
    ...(input.object ? { object: input.object } : {}),
    file: input.file,
  });
  return asProfile(payload);
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
  // The refused save's edits lose to the other tab: the screen shows what the
  // run holds and nothing is re-sent over it.
  const writes = useDraftWrites(() => {
    latest.current = NO_EDITS;
    setEditsState(NO_EDITS);
  });
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
      const profile = await postSource(input);
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
      edit((current) => ({
        ...NO_EDITS,
        step: "files",
        ...(input.retype
          ? {
              sourceName: current.sourceName,
              contextTagID: current.contextTagID,
            }
          : null),
      }));
      setRunId(profile.run_id);
    },
  });

  const profile = run.data?.profile ?? upload.data?.profile ?? null;
  const zip = profile?.parts !== undefined;
  const files = profile ? filesOf(profile) : [];
  const proposedName =
    upload.data?.name ?? profile?.suggested_source_name ?? "";
  const choices = choicesOf(files, zip, run.data, edits, proposedName);
  const draftOpen =
    profile !== null && (status === undefined || status === "pending");

  // Edits settle for a moment, then go to the server as one save.
  const settled = useSettledValue(edits);
  const sent = useRef(NO_EDITS);
  const toSave = editsPatch(
    files,
    zip,
    choicesOf(files, zip, run.data, settled, proposedName),
    settled,
  );
  const { save } = writes;
  useEffect(() => {
    if (settled === sent.current || runId === null || !draftOpen) return;
    sent.current = settled;
    if (toSave) save(runId, toSave);
  }, [settled, runId, draftOpen, toSave, save]);

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
    onSettled: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });

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
  const draftStep =
    edits.step ?? run.data?.draft_step ?? (run.data ? "map" : "files");
  const step = runId === null ? "upload" : stepOfRun(effective, draftStep);
  const anyway = test.isPending
    ? test.variables?.patch.on_broken_link === "drop_link"
    : run.data?.on_broken_link === "drop_link";

  const reset = () => {
    generation.current += 1;
    upload.reset();
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
    // A ZIP part's type changes in place, and its mapping starts again from
    // that type's proposal. A single CSV is profiled for one type, so a new
    // type uploads the same file again.
    setFileObject: (file: string, object: ImportObject | null) => {
      if (profile && !zip && source && object) {
        upload.mutate({
          file: source,
          object,
          recent: undefined,
          retype: true,
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
      cancelRun.mutate(runId, {
        // Cancelling a draft another tab is testing only stops that test and
        // leaves the draft, so the page stays on it until it is cancelled.
        onSuccess: (stopped) => {
          if (stopped?.status !== "cancelled") return;
          reset();
          navigate(settingsHref("import"));
        },
      });
    },
    cancelling: cancelRun,
    reset,
  };
}

export type ImportWizardState = ReturnType<typeof useImportWizard>;
