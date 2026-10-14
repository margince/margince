// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ProblemError, unwrap } from "../lib/problem";
import { isRecord } from "../lib/problembody";
import { mappedOnly, type WizardFile } from "./importflow";
import { IMPORTS_KEY } from "./importrun";
import type { ImportObject, ImportRun } from "./importtypes";

type Mapping = Readonly<Record<string, string>>;

/** The choices a save writes onto a draft; the version is the queue's to add. */
export type DraftPatch = Omit<
  components["schemas"]["UpdateImportRunRequest"],
  "version"
>;

/**
 * What the reader changed since the run last said what it holds. The run is
 * the source of every other value; an edit shows until the run answers.
 */
export type Edits = Readonly<{
  step?: "files" | "map";
  objects: Readonly<Record<string, ImportObject | null>>;
  mapping: Readonly<Record<string, Mapping>>;
  sourceName?: string;
  contextTagID?: string;
}>;

export const NO_EDITS: Edits = { objects: {}, mapping: {} };

export type Choices = Readonly<{
  objects: Readonly<Record<string, ImportObject | null>>;
  mapping: Readonly<Record<string, Mapping>>;
  sourceName: string;
  contextTagID: string;
}>;

// The type the run holds for a file. A single CSV's file name may be empty on
// the run, so its one type is the run's own.
function savedObject(
  run: ImportRun | undefined,
  file: WizardFile,
  zip: boolean,
): ImportObject | undefined {
  if (!zip) return run?.object ?? run?.parts?.[0]?.object;
  return run?.parts?.find((part) => part.file === file.file)?.object;
}

// The mapping the run saved for a file. It holds only for the type it was
// saved under, because another type's columns land on other fields.
function savedMapping(
  run: ImportRun | undefined,
  file: WizardFile,
  zip: boolean,
  object: ImportObject | undefined,
): Mapping | undefined {
  const part = zip
    ? run?.parts?.find((p) => p.file === file.file)
    : run?.parts?.[0];
  return part?.object === object ? part?.mapping : undefined;
}

/** The run's choices with the reader's edits laid over them. */
export function choicesOf(
  files: readonly WizardFile[],
  zip: boolean,
  run: ImportRun | undefined,
  edits: Edits,
  proposedName: string,
): Choices {
  const objects: Record<string, ImportObject | null> = {};
  const mapping: Record<string, Mapping> = {};
  for (const file of files) {
    const object =
      file.file in edits.objects
        ? edits.objects[file.file]
        : (savedObject(run, file, zip) ?? file.detected);
    objects[file.file] = object ?? null;
    mapping[file.file] =
      edits.mapping[file.file] ??
      savedMapping(run, file, zip, object ?? undefined) ??
      (object ? file.suggested[object] : undefined) ??
      {};
  }
  return {
    objects,
    mapping,
    sourceName: edits.sourceName ?? (run?.source_name || proposedName),
    contextTagID: edits.contextTagID ?? run?.context_tag_id ?? "",
  };
}

// One CSV is its object and mapping; a ZIP is one part per typed file. A
// choice nobody typed is left out, never sent empty. Null when no file is typed
// yet, because the server refuses a draft with nothing to import.
function filesPatch(
  files: readonly WizardFile[],
  zip: boolean,
  choices: Choices,
): DraftPatch | null {
  const typed = files.flatMap((file) => {
    const object = choices.objects[file.file];
    return object
      ? [
          {
            file: file.file,
            object,
            mapping: mappedOnly(choices.mapping[file.file] ?? {}),
          },
        ]
      : [];
  });
  const single = zip ? undefined : typed[0];
  if (single) return { object: single.object, mapping: single.mapping };
  return zip && typed.length > 0 ? { parts: typed } : null;
}

function namePatch(choices: Choices): DraftPatch {
  const name = choices.sourceName.trim();
  return name === "" ? {} : { source_name: name };
}

// An empty tag clears the tag the run holds; with none held there is nothing
// to write.
function tagPatch(choices: Choices, held: boolean): DraftPatch {
  if (choices.contextTagID !== "") {
    return { context_tag_id: choices.contextTagID };
  }
  return held ? { context_tag_id: null } : {};
}

/** Everything the draft holds, written when the reader asks for the test. */
export function fullPatch(
  files: readonly WizardFile[],
  zip: boolean,
  choices: Choices,
  run: ImportRun | undefined,
): DraftPatch {
  return {
    ...filesPatch(files, zip, choices),
    ...namePatch(choices),
    ...tagPatch(choices, Boolean(run?.context_tag_id)),
    draft_step: "map",
  };
}

/** Only what the edits touched, or null when they changed nothing to save. */
export function editsPatch(
  files: readonly WizardFile[],
  zip: boolean,
  choices: Choices,
  edits: Edits,
): DraftPatch | null {
  const touchedFiles =
    Object.keys(edits.objects).length > 0 ||
    Object.keys(edits.mapping).length > 0;
  const patch: DraftPatch = {
    ...(touchedFiles ? filesPatch(files, zip, choices) : null),
    ...(edits.sourceName === undefined ? null : namePatch(choices)),
    ...(edits.contextTagID === undefined ? null : tagPatch(choices, true)),
    ...(edits.step ? { draft_step: edits.step } : null),
  };
  return Object.keys(patch).length > 0 ? patch : null;
}

function isConflict(error: unknown): boolean {
  return (
    error instanceof ProblemError &&
    isRecord(error.problem) &&
    error.problem.status === 409
  );
}

/**
 * The one line every save of a draft goes through.
 * Saves run one at a time, and each carries the version the previous answer gave.
 * So two quick edits never race each other into a conflict.
 * A conflict can only come from another tab.
 */
export function useDraftWrites(onConflict?: () => void) {
  const queryClient = useQueryClient();
  const tail = useRef<Promise<unknown>>(Promise.resolve());
  const carried = useRef<{ id: string; version: number } | null>(null);
  const [failure, setFailure] = useState<unknown>(null);
  const [conflict, setConflict] = useState(false);

  const send = async (id: string, patch: DraftPatch): Promise<ImportRun> => {
    const key = [IMPORTS_KEY, "run", id];
    const cached = queryClient.getQueryData<ImportRun>(key);
    const known = carried.current?.id === id ? carried.current.version : 0;
    const version = Math.max(cached?.version ?? 0, known);
    const saved = unwrap(
      await api.PATCH("/imports/{id}", {
        params: { path: { id } },
        body: { ...patch, version },
      }),
    );
    carried.current = { id, version: saved.version };
    queryClient.setQueryData<ImportRun>(key, (old) => ({
      ...old,
      ...saved,
      profile: saved.profile ?? old?.profile,
    }));
    return saved;
  };

  // Rejects for a caller that must know (the test run). The chain itself
  // never stays rejected, so one refused save does not refuse the next.
  const patch = (id: string, body: DraftPatch): Promise<ImportRun> => {
    const run = tail.current.then(() => send(id, body));
    tail.current = run.then(
      () => undefined,
      () => undefined,
    );
    return run.catch((error: unknown) => {
      if (isConflict(error)) {
        setConflict(true);
        onConflict?.();
        queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] });
      }
      throw error;
    });
  };

  return {
    patch,
    /** An edit saved in the background; a refusal waits in `failure`. */
    save: (id: string, body: DraftPatch) => {
      setFailure(null);
      patch(id, body).catch(setFailure);
    },
    failure,
    conflict,
    clearConflict: () => setConflict(false),
  };
}
