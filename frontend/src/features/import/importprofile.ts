// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ImportPartProfile, ImportProfile } from "./importtypes";
import { isImportObject } from "./importtypes";

// asProfile checks the one payload nothing else types. The upload is a
// hand-rolled multipart fetch (the generated client cannot send a file part).
// So its answer arrives as parsed JSON with no shape behind it. A cast cannot
// be kept, so each member the screen reads is checked and rebuilt.
export function asProfile(payload: unknown): ImportProfile {
  const read = reader(payload);
  const run_id = read("run_id");
  const source_ref = read("source_ref");
  const object = read("object");
  const rows_profiled = read("rows_profiled");
  const row_count = read("row_count");
  const columns = read("columns");
  const targets = read("targets");
  const suggested_mapping = read("suggested_mapping");
  const parts = read("parts");
  if (
    typeof run_id !== "string" ||
    typeof source_ref !== "string" ||
    typeof rows_profiled !== "number" ||
    typeof row_count !== "number" ||
    !Array.isArray(columns) ||
    !Array.isArray(targets) ||
    !(object === undefined || isImportObject(object)) ||
    !isMapping(suggested_mapping) ||
    !(parts === undefined || Array.isArray(parts))
  ) {
    throw new Error(unreadableUpload);
  }
  return {
    run_id,
    source_ref,
    object,
    rows_profiled,
    row_count,
    columns,
    targets,
    suggested_mapping,
    file_name: optionalText(read("file_name")),
    suggested_source_name: optionalText(read("suggested_source_name")),
    parts: parts?.map(asPart),
    ignored: asIgnored(read("ignored")),
  };
}

// Read by name rather than destructured through a cast. The payload is
// unknown here, and these readers exist to avoid asserting a shape onto it.
function reader(payload: unknown): (name: string) => unknown {
  if (typeof payload !== "object" || payload === null) {
    throw new Error(unreadableUpload);
  }
  return (name) => (name in payload ? Reflect.get(payload, name) : undefined);
}

function optionalText(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function asPart(payload: unknown): ImportPartProfile {
  const read = reader(payload);
  const file = read("file");
  const object = read("object");
  const columns = read("columns");
  const rows_profiled = read("rows_profiled");
  const row_count = read("row_count");
  if (
    typeof file !== "string" ||
    !isImportObject(object) ||
    !Array.isArray(columns) ||
    typeof rows_profiled !== "number" ||
    typeof row_count !== "number"
  ) {
    throw new Error(unreadableUpload);
  }
  return {
    file,
    object,
    columns,
    rows_profiled,
    row_count,
    suggested_mappings: asMappings(read("suggested_mappings")),
  };
}

// One proposed mapping per record type, keyed by the type.
function asMappings(value: unknown): Record<string, Record<string, string>> {
  if (typeof value !== "object" || value === null) {
    throw new Error(unreadableUpload);
  }
  const mappings: Record<string, Record<string, string>> = {};
  for (const [object, mapping] of Object.entries(value)) {
    if (!isMapping(mapping)) throw new Error(unreadableUpload);
    mappings[object] = mapping;
  }
  return mappings;
}

function asIgnored(value: unknown): ImportProfile["ignored"] {
  if (value === undefined) return undefined;
  if (!Array.isArray(value)) throw new Error(unreadableUpload);
  return value.map((entry) => {
    const read = reader(entry);
    const file = read("file");
    const reason = read("reason");
    if (typeof file !== "string" || reason !== "unknown_name") {
      throw new Error(unreadableUpload);
    }
    return { file, reason };
  });
}

const unreadableUpload =
  "the upload answered something this screen cannot read";

function isMapping(value: unknown): value is Record<string, string> {
  return (
    typeof value === "object" &&
    value !== null &&
    Object.values(value).every((entry) => typeof entry === "string")
  );
}
