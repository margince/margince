// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { Locale } from "../i18n";
import { throwProblem, unwrap } from "./common";
import { useAddressedRun } from "./importaddress";
import { asProfile } from "./importprofile";
import type {
  ImportFormat,
  ImportObject,
  ImportProfile,
  ImportRun,
} from "./importtypes";
import { DONT_IMPORT, UNNAMED_IMPORT_OBJECT } from "./importtypes";

// Every read of the import surface sits under this key. One invalidation after
// a write then refreshes the run, its report and the recent list together.
const IMPORTS_KEY = "imports";

// The files an import takes, their columns and their order, in the reader's
// language. The template preview, the guide and every screen that orders
// types read it, so none of them keeps a list of its own.
export function useImportFormat(lang: Locale) {
  return useQuery({
    queryKey: [IMPORTS_KEY, "format", lang],
    queryFn: async () =>
      unwrap(await api.GET("/imports/format", { params: { query: { lang } } })),
    staleTime: 60_000,
  });
}

// A run that is still moving under the server's hand. The commit and the undo
// both run detached, so only a read tells the screen when they finish.
function moving(status: ImportRun["status"] | undefined): boolean {
  return status === "running" || status === "undoing";
}

const POLL_MS = 2000;

// The newest runs first, as the server lists them. Polled while one of them is
// still moving, so the list shows the end of a run the reader walked away from.
export function useRecentImports() {
  return useQuery({
    queryKey: [IMPORTS_KEY, "recent"],
    queryFn: async () =>
      unwrap(await api.GET("/imports", { params: { query: { limit: 20 } } }))
        .data,
    refetchInterval: (query) =>
      query.state.data?.some((one) => moving(one.status)) ? POLL_MS : false,
  });
}

// One run, read again every two seconds while it is moving.
export function useImportRun(id: string | null) {
  return useQuery({
    queryKey: [IMPORTS_KEY, "run", id],
    queryFn:
      id === null
        ? skipToken
        : async () =>
            unwrap(
              await api.GET("/imports/{id}", { params: { path: { id } } }),
            ),
    refetchInterval: (query) =>
      moving(query.state.data?.status) ? POLL_MS : false,
  });
}

// Starts the reversal of one run. It answers at once with the run `undoing`;
// the reads above follow it to `undone`.
export function useUndoImport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.POST("/imports/{id}/undo", { params: { path: { id } } }),
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });
}

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
  // Multipart by hand: the generated client serializes JSON bodies, and this
  // operation takes a file part.
  const body = new FormData();
  if (input.object) body.append("object", input.object);
  body.append("file", input.file);
  // contract-fetch:allow multipart: see the note above
  const response = await fetch("/v1/imports/sources", {
    method: "POST",
    body,
    credentials: "include",
  });
  const payload = await response.json().catch(() => undefined);
  if (!response.ok) {
    throwProblem(payload);
  }
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

// One run's report: the test run's prediction, then what the run did. Keyed by
// the run's status, so each move of the run reads it again.
export function useImportReport(
  id: string | null,
  status: ImportRun["status"] | undefined,
) {
  return useQuery({
    queryKey: [IMPORTS_KEY, "report", id, status],
    queryFn:
      id === null || status === undefined
        ? skipToken
        : async () =>
            unwrap(
              await api.GET("/imports/{id}/report", {
                params: { path: { id } },
              }),
            ),
  });
}

// Which step a run stands on. A staged run is tested; a moving or stopped one
// is importing; the rest have a result. A run still being read is tested,
// the step it was staged on.
function stepOfRun(status: ImportRun["status"] | undefined): WizardStep {
  switch (status) {
    case "running":
    case "failed":
      return "import";
    case "complete":
    case "undoing":
    case "undone":
      return "result";
    default:
      return "test";
  }
}

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
  // Whether any run was staged from this upload. It outlives a failed restage,
  // which leaves the earlier run on the server.
  const [staged, setStaged] = useState(false);
  const [runId, setRunId] = useAddressedRun();
  const run = useImportRun(runId);
  const status = run.data?.status;
  const report = useImportReport(runId, status);
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
      setRunId(run.id);
      queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] });
    },
  });

  const approve = useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.POST("/imports/{id}/approve", { params: { path: { id } } }),
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });

  const files = profile ? filesOf(profile) : [];
  const step = runId === null ? phase : stepOfRun(status);
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
    approve,
    undo,
    stageInput,
    anyway,
    // Rows with broken links come in without the link, or stay out. The
    // choice stages the run again, so the report approved is the one shown.
    setAnyway: (next: boolean) => {
      setAnyway(next);
      if (profile) stage.mutate(stageInput(profile, next));
    },
    goTo: (next: "files" | "map") => setPhase(next),
    reset: () => {
      generation.current += 1;
      upload.reset();
      stage.reset();
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

// A run that lands or reverses wrote records any cached list may hold. Every
// read is asked again once the runs watched have all settled.
export function useLandedRecords(
  statuses: readonly (ImportRun["status"] | undefined)[],
): void {
  const queryClient = useQueryClient();
  const now = statuses.some(moving);
  const seen = useRef(now);
  useEffect(() => {
    const before = seen.current;
    seen.current = now;
    if (before && !now) {
      queryClient.invalidateQueries();
    }
  }, [now, queryClient]);
}
