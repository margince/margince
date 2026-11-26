// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Choices, Edits } from "./importdraft";
import { filesOf, type WizardFile } from "./importflow";
import type { ImportObject, ImportProfile } from "./importtypes";

type Mapping = Readonly<Record<string, string>>;

type CarriedFile = Readonly<{
  file: string;
  headers: readonly string[];
  object: ImportObject | null;
  mapping: Mapping;
}>;

/** What a tested run chose, kept for the file uploaded in its place. */
export type CarriedChoices = Readonly<{
  zip: boolean;
  files: readonly CarriedFile[];
  sourceName: string;
  contextTagID: string;
}>;

const headersOf = (file: WizardFile) => file.columns.map((one) => one.header);

/** The choices to keep, or null for a run whose file is no longer read. */
export function choicesToCarry(
  files: readonly WizardFile[],
  zip: boolean,
  choices: Choices,
): CarriedChoices | null {
  if (files.length === 0) return null;
  return {
    zip,
    files: files.map((file) => ({
      file: file.file,
      headers: headersOf(file),
      object: choices.objects[file.file] ?? null,
      mapping: choices.mapping[file.file] ?? {},
    })),
    sourceName: choices.sourceName,
    contextTagID: choices.contextTagID,
  };
}

function sameHeaderSet(a: readonly string[], b: readonly string[]): boolean {
  const want = new Set(a);
  return want.size === new Set(b).size && b.every((one) => want.has(one));
}

// Each file of the upload with the kept file it stands in for: the one CSV, or
// the ZIP part of the same name. Null unless every file has the same headers.
function pairsOf(
  carried: CarriedChoices,
  profile: ImportProfile,
): readonly (readonly [WizardFile, CarriedFile])[] | null {
  const files = filesOf(profile);
  const zip = profile.parts !== undefined;
  if (zip !== carried.zip || files.length !== carried.files.length) return null;
  const pairs: (readonly [WizardFile, CarriedFile])[] = [];
  for (const file of files) {
    const was = zip
      ? carried.files.find((one) => one.file === file.file)
      : carried.files[0];
    if (!was || !sameHeaderSet(was.headers, headersOf(file))) return null;
    pairs.push([file, was]);
  }
  return pairs;
}

/**
 * The type a single CSV has to be uploaded as again, so it is read as the type
 * the replaced run had. Undefined when it was read as that type, or nothing is
 * carried.
 */
export function carriedObject(
  carried: CarriedChoices,
  profile: ImportProfile,
): ImportObject | undefined {
  const was = carried.zip ? undefined : pairsOf(carried, profile)?.[0]?.[1];
  return was?.object && was.object !== profile.object ? was.object : undefined;
}

/**
 * The edits an upload starts from when it replaces a tested run with the same
 * headers: that run's types, mapping, source name and tag. Null when the
 * headers differ, so the upload starts from the server's proposal.
 */
export function carriedEdits(
  carried: CarriedChoices,
  profile: ImportProfile,
): Edits | null {
  const pairs = pairsOf(carried, profile);
  if (!pairs) return null;
  const objects: Record<string, ImportObject | null> = {};
  const mapping: Record<string, Mapping> = {};
  for (const [file, was] of pairs) {
    // A ZIP part changes type in place. A single CSV was read as one type, and
    // a mapping holds only for the type it was made under.
    if (carried.zip) objects[file.file] = was.object;
    if (carried.zip || was.object === profile.object) {
      mapping[file.file] = was.mapping;
    }
  }
  return {
    objects,
    mapping,
    ...(carried.sourceName ? { sourceName: carried.sourceName } : null),
    ...(carried.contextTagID ? { contextTagID: carried.contextTagID } : null),
  };
}
