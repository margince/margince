// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { requireVersion } from "../api/version";
import { isVersionSkewOf } from "./common";

type Values = Record<string, unknown>;
type Versioned = { id: string; version?: number; masked_fields?: string[] };
type Change = { path: string[]; before: unknown; after: unknown };

function object(value: unknown): value is Values {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

// Object order carries no meaning; array order does (primary email and phone
// positions, for example). Missing optional values read as JSON null.
export function sameEditValue(a: unknown, b: unknown): boolean {
  if (a == null && b == null) return true;
  if (Array.isArray(a) && Array.isArray(b)) {
    return a.length === b.length && a.every((v, i) => sameEditValue(v, b[i]));
  }
  if (object(a) && object(b)) {
    return [...new Set([...Object.keys(a), ...Object.keys(b)])].every((key) =>
      sameEditValue(a[key], b[key]),
    );
  }
  return a === b;
}

function changesFrom(
  patch: Values,
  before: Values,
  prefix: string[] = [],
): Change[] {
  return Object.entries(patch).flatMap(([key, after]) => {
    if (after === undefined || sameEditValue(before[key], after)) return [];
    const path = [...prefix, key];
    if (object(after))
      return changesFrom(after, object(before[key]) ? before[key] : {}, path);
    return [{ path, before: before[key], after }];
  });
}

function at(values: Values, path: string[]): unknown {
  let value: unknown = values;
  for (const key of path) value = object(value) ? value[key] : undefined;
  return value;
}

function patchOnto(latest: Values, changes: Change[]): Values {
  const patch: Values = {};
  for (const { path, after } of changes) {
    let target = patch;
    let source = latest;
    for (const key of path.slice(0, -1)) {
      // Nested API objects replace their value as a whole. Carry the latest
      // untouched members, including members this form does not render.
      source = object(source[key]) ? source[key] : {};
      target[key] ??= structuredClone(source);
      target = target[key] as Values;
    }
    target[path[path.length - 1]] = after;
  }
  return patch;
}

function originalRecord<T>(opened: (Values & { id: string }) | undefined): T {
  if (!object(opened?.original))
    throw new Error("The edit has no original record.");
  // Custom fields can arrive after the dialog opens. Its seeding machinery
  // records their actual baseline alongside the opening record, so use it.
  const original = { ...opened.original } as T;
  for (const [key, value] of Object.entries(opened)) {
    if (key.startsWith("cf_")) (original as Values)[key] = value;
  }
  return original;
}

// Retry ONLY a definite version refusal, never a timeout or lost response.
// Every attempt retains If-Match: another write between the fresh read and
// our PATCH must fail again rather than silently losing that newer change.
export async function saveIndependentEdit<T extends Versioned>({
  opened,
  patch,
  read,
  write,
  project = (record) => ({ ...record }),
  groups = [],
}: {
  opened: (Values & { id: string }) | undefined;
  patch: Values;
  read: () => Promise<T>;
  write: (patch: Values, version: number) => Promise<T>;
  project?: (record: T) => Values;
  groups?: readonly (readonly string[])[];
}): Promise<T> {
  const original = originalRecord<T>(opened);
  requireVersion(original.version);
  const before = project(original);
  const changes = changesFrom(patch, before);
  if (changes.length === 0) return original;
  const touched = new Set(changes.map(({ path }) => path[0]));
  const dependencies = groups
    .filter((group) => group.some((key) => touched.has(key)))
    .flat();
  retainSuppliedPeers(changes, patch, before, dependencies, touched);
  let current = original;
  for (let attempt = 0; ; attempt++) {
    try {
      return await write(
        patchOnto(project(current), changes),
        requireVersion(current.version),
      );
    } catch (error) {
      if (!isVersionSkewOf(error) || attempt === 2) throw error;
      const latest = await read();
      const fresh = project(latest);
      if (
        latest.id !== original.id ||
        latest.masked_fields?.some(
          (key) => touched.has(key) || dependencies.includes(key),
        ) ||
        changes.some(
          ({ path, before: value }) => !sameEditValue(value, at(fresh, path)),
        ) ||
        dependencies.some((key) => !sameEditValue(before[key], fresh[key]))
      )
        throw error;
      current = latest;
    }
  }
}

// Currency and amount travel together even when the numeric amount is equal.
function retainSuppliedPeers(
  changes: Change[],
  patch: Values,
  before: Values,
  dependencies: string[],
  touched: Set<string>,
) {
  for (const key of new Set(dependencies)) {
    if (patch[key] !== undefined && !touched.has(key)) {
      changes.push({ path: [key], before: before[key], after: patch[key] });
    }
  }
}
