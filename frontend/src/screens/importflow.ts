// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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
export function mappedOnly(
  mapping: Record<string, string>,
): Record<string, string> {
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
