// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../api/client";
import { postMultipart } from "../lib/multipart";
import { unwrap } from "../lib/problem";
import { useAddressedRun } from "./importaddress";
import { asProfile } from "./importprofile";
import {
  IMPORTS_KEY,
  useImportReport,
  useImportRun,
  useLandedRecords,
  useUndoImport,
} from "./importrun";
import type {
  ImportFormat,
  ImportObject,
  ImportProfile,
  ImportRun,
} from "./importtypes";
import { DONT_IMPORT, UNNAMED_IMPORT_OBJECT } from "./importtypes";

// The file the hub's drop zone took, held for the run page to start with. A
// File cannot ride an address, and the two are separate routes.
let handedOver: File | null = null;

export function handOverUpload(file: File): void {
  handedOver = file;
}

export function takeHandedOverUpload(): File | null {
  const file = handedOver;
  handedOver = null;
  return file;
}

// mappedOnly drops the columns the human left on "don't import": the wire
// carries the mapping they chose, not every column the file happened to have.
function mappedOnly(mapping: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [column, target] of Object.entries(mapping)) {
    if (target && target !== DONT_IMPORT) {
      out[column] = target;
    }
  }
  return out;
}

/** Where a reader is in the run flow; the first three come before any run. */
export type WizardStep =
  | "upload"
  | "files"
  | "map"
  | "test"
  | "import"
  | "result";

type Mapping = Readonly<Record<string, string>>;

/** One file of an upload, as the Files and Map steps work on it. */
export type WizardFile = Readonly<{
  file: string;
  rowsProfiled: number;
  columns: ImportProfile["columns"];
  /** The type the server detected, null where it detected none. */
  detected: ImportObject | null;
  /** One proposed mapping per type, so a changed type needs no second call. */
  suggested: Readonly<Record<string, Mapping>>;
}>;

// A ZIP is its parts. A single CSV is one file whose only proposal is for the
// type it was uploaded as.
export function filesOf(profile: ImportProfile): WizardFile[] {
  if (profile.parts) {
    return profile.parts.map((part) => ({
      file: part.file,
      rowsProfiled: part.rows_profiled,
      columns: part.columns,
      detected: part.object ?? null,
      suggested: part.suggested_mappings,
    }));
  }
  return [
    {
      file: profile.file_name ?? "",
      rowsProfiled: profile.rows_profiled,
      columns: profile.columns,
      detected: profile.object ?? null,
      suggested: profile.object
        ? { [profile.object]: profile.suggested_mapping }
        : {},
    },
  ];
}

const ZIP_NAME = /\.zip$/i;

// The type a single CSV is uploaded as: the format's file name for a type when
// the file carries one, else the flow's default. A ZIP names its types itself.
export function uploadObjectFor(
  name: string,
  format: ImportFormat | undefined,
): ImportObject | undefined {
  if (ZIP_NAME.test(name)) return undefined;
  const lower = name.toLowerCase();
  return (
    format?.files.find((file) => file.file === lower)?.object ??
    UNNAMED_IMPORT_OBJECT
  );
}

// A ZIP starts with the source name the last ZIP run was filed under, else the
// server's proposal. So a re-downloaded "crm (1).zip" updates what "crm.zip"
// landed rather than starting a second namespace. A single CSV keeps
// the server's empty proposal, so its re-upload stays where it was first filed.
export function defaultSourceName(
  profile: ImportProfile,
  recent: readonly ImportRun[] | undefined,
): string {
  if (!profile.parts) return profile.suggested_source_name ?? "";
  const last = recent?.find(
    (one) => ZIP_NAME.test(one.file_name ?? "") && one.source_name,
  );
  return last?.source_name ?? profile.suggested_source_name ?? "";
}

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

type StageInput = Readonly<{
  profile: ImportProfile;
  objects: Readonly<Record<string, ImportObject | null>>;
  mapping: Readonly<Record<string, Mapping>>;
  sourceName: string;
  onBrokenLink: "skip_row" | "drop_link";
  contextTagID: string;
}>;

// The create request for the files the reader typed: parts for a ZIP, the
// object and mapping for one CSV. Empty choices are left out, not sent empty.
function stageBody(input: StageInput) {
  const typed = filesOf(input.profile).flatMap((file) => {
    const object = input.objects[file.file];
    return object
      ? [
          {
            file: file.file,
            object,
            mapping: mappedOnly(input.mapping[file.file] ?? {}),
          },
        ]
      : [];
  });
  const single = input.profile.parts ? undefined : typed[0];
  return {
    connector: "csv" as const,
    source_ref: input.profile.source_ref,
    ...(single
      ? { object: single.object, mapping: single.mapping }
      : { parts: typed }),
    ...(input.sourceName.trim() === ""
      ? {}
      : { source_name: input.sourceName.trim() }),
    on_broken_link: input.onBrokenLink,
    ...(input.profile.file_name ? { file_name: input.profile.file_name } : {}),
    ...(input.contextTagID === ""
      ? {}
      : { context_tag_id: input.contextTagID }),
  };
}

// Which step a run stands on. A draft stands on the step its reader is at; a
// cancelled run is done with, so the reader starts again from an upload. A run
// still being read is tested, the step it was staged on.
export function stepOfRun(
  status: ImportRun["status"] | undefined,
  draftStep: "files" | "map",
): WizardStep {
  switch (status) {
    case "pending":
      return draftStep;
    case "cancelled":
      return "upload";
    case "running":
    case "failed":
      return "import";
    case "complete":
    case "undoing":
    case "undone":
      return "result";
    case "validating":
    case "awaiting_approval":
    case undefined:
      return "test";
  }
}

type Retest = Readonly<{
  id: string;
  version: number;
  onBrokenLink: "skip_row" | "drop_link";
}>;

// The run flow: upload, check the files, map their columns, stage a test run,
// import it and read the result. Every answer carries the generation it was
// asked in, and an answer from a generation the reader has since left is
// dropped. The run itself lives in the address, so a reader who leaves comes
// back to it.
export function useImportWizard() {
  const queryClient = useQueryClient();
  const generation = useRef(0);
  const [phase, setPhase] = useState<"upload" | "files" | "map">("upload");
  const [source, setSource] = useState<File | null>(null);
  const [profile, setProfile] = useState<ImportProfile | null>(null);
  const [objects, setObjects] = useState<
    Readonly<Record<string, ImportObject | null>>
  >({});
  const [mapping, setMapping] = useState<Readonly<Record<string, Mapping>>>({});
  const [sourceName, setSourceName] = useState("");
  const [contextTagID, setContextTagID] = useState("");
  const [anyway, setAnyway] = useState(false);
  // Whether any run was staged from this upload. It outlives a failed retest,
  // which leaves the run on the server.
  const [staged, setStaged] = useState(false);
  const [runId, setRunId] = useAddressedRun();
  const run = useImportRun(runId);
  const status = run.data?.status;
  const report = useImportReport(runId, status, run.data?.version);
  const undo = useUndoImport();
  useLandedRecords([status]);

  const upload = useMutation({
    mutationFn: async (input: UploadInput) => {
      const at = generation.current;
      return { at, input, profile: await postSource(input) };
    },
    onSuccess: ({ at, input, profile: next }) => {
      if (at !== generation.current) return;
      const files = filesOf(next);
      setSource(input.file);
      setProfile(next);
      setObjects(Object.fromEntries(files.map((f) => [f.file, f.detected])));
      setMapping(
        Object.fromEntries(
          files.map((f) => [
            f.file,
            f.detected ? (f.suggested[f.detected] ?? {}) : {},
          ]),
        ),
      );
      if (!input.retype) setSourceName(defaultSourceName(next, input.recent));
      setPhase("files");
    },
  });

  const stage = useMutation({
    mutationFn: async (input: StageInput) => {
      const at = generation.current;
      const run = unwrap(
        await api.POST("/imports", { body: stageBody(input) }),
      );
      return { at, run };
    },
    onSuccess: ({ at, run }) => {
      if (at !== generation.current) return;
      setStaged(true);
      retest.reset();
      setRunId(run.id);
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });

  // The same run tested again with the one choice changed. The version comes
  // from the save's answer, so the test reads the save it follows.
  const retest = useMutation({
    mutationFn: async ({ id, version, onBrokenLink }: Retest) => {
      const path = { params: { path: { id } } };
      const saved = unwrap(
        await api.PATCH("/imports/{id}", {
          ...path,
          body: { version, on_broken_link: onBrokenLink },
        }),
      );
      return unwrap(
        await api.POST("/imports/{id}/validate", {
          ...path,
          body: { version: saved.version },
        }),
      );
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });

  // The version is the one the reader's report was read at, so a run that
  // changed since is refused rather than imported unread. Every write re-reads
  // on refusal too, so the next attempt carries the version the server holds.
  const approve = useMutation({
    mutationFn: async ({ id, version }: Pick<ImportRun, "id" | "version">) =>
      unwrap(
        await api.POST("/imports/{id}/approve", {
          params: { path: { id } },
          body: { version },
        }),
      ),
    onSettled: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });

  const files = profile ? filesOf(profile) : [];
  const draftStep =
    phase === "upload" ? (run.data?.draft_step ?? "map") : phase;
  const step = runId === null ? phase : stepOfRun(status, draftStep);
  const stageInput = (
    staged: ImportProfile,
    dropLinks: boolean = anyway,
  ): StageInput => ({
    profile: staged,
    objects,
    mapping,
    sourceName,
    onBrokenLink: dropLinks ? "drop_link" : "skip_row",
    contextTagID,
  });

  return {
    step,
    profile,
    files,
    objects,
    // A ZIP part's type changes in place, and its mapping starts again from
    // that type's proposal. A single CSV is profiled for one type, so a new
    // type uploads the same file again.
    setFileObject: (file: string, object: ImportObject | null) => {
      if (profile && !profile.parts && source && object) {
        upload.mutate({
          file: source,
          object,
          recent: undefined,
          retype: true,
        });
        return;
      }
      setObjects((current) => ({ ...current, [file]: object }));
      const suggested = files.find((one) => one.file === file)?.suggested;
      setMapping((current) => ({
        ...current,
        [file]: object ? (suggested?.[object] ?? {}) : {},
      }));
    },
    mapping,
    setColumnTarget: (file: string, column: string, target: string) =>
      setMapping((current) => ({
        ...current,
        [file]: { ...current[file], [column]: target },
      })),
    sourceName,
    setSourceName,
    contextTagID,
    setContextTagID,
    runId,
    run,
    report,
    upload,
    stage,
    staged,
    retest,
    // The files and the map open again while the run is a draft whose upload
    // this page holds. A staged run was made from them as they were.
    editable: runId === null || (status === "pending" && profile !== null),
    approve,
    undo,
    stageInput,
    anyway,
    // Rows with broken links come in without the link, or stay out. The
    // choice tests the run again, so the report approved is the one shown.
    setAnyway: (next: boolean) => {
      const current = run.data;
      if (!current) return;
      setAnyway(next);
      retest.mutate({
        id: current.id,
        version: current.version,
        onBrokenLink: next ? "drop_link" : "skip_row",
      });
    },
    goTo: (next: "files" | "map") => setPhase(next),
    reset: () => {
      generation.current += 1;
      upload.reset();
      stage.reset();
      retest.reset();
      approve.reset();
      undo.reset();
      setAnyway(false);
      setStaged(false);
      setPhase("upload");
      setSource(null);
      setProfile(null);
      setObjects({});
      setMapping({});
      setSourceName("");
      setContextTagID("");
      setRunId(null);
    },
  };
}

export type ImportWizardState = ReturnType<typeof useImportWizard>;
