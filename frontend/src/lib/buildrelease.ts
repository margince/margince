// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  BUILD_REVISION,
  CORE_RELEASE,
  comparableRelease,
  SPA_RELEASE,
  shortRevision,
} from "./release";

export const REPOSITORY = "https://github.com/margince/margince";
export const RELEASE_NOTES_HREF = `${REPOSITORY}/releases`;

/**
 * What this bundle knows of its build. A stamped core release marks an instance
 * build; its own release, which may be unstamped, and its `revision` are the
 * instance repository's. `revision` is a full, comparable commit or undefined.
 */
export type BuildRelease =
  | Readonly<{
      kind: "instance";
      core: string;
      instance?: string;
      revision?: string;
    }>
  | Readonly<{ kind: "core"; release: string; revision?: string }>
  | Readonly<{ kind: "unstamped"; revision?: string }>;

export function buildRelease(): BuildRelease {
  const revision = comparableRelease(BUILD_REVISION)
    ? BUILD_REVISION
    : undefined;
  if (comparableRelease(CORE_RELEASE)) {
    return {
      kind: "instance",
      core: CORE_RELEASE,
      instance: comparableRelease(SPA_RELEASE) ? SPA_RELEASE : undefined,
      revision,
    };
  }
  return comparableRelease(SPA_RELEASE)
    ? { kind: "core", release: SPA_RELEASE, revision }
    : { kind: "unstamped", revision };
}

/**
 * The release a problem report names. The tracker is the core's, so an instance
 * build names the core release it pins, and an unstamped one its commit.
 */
export function reportedRelease(build: BuildRelease): string | undefined {
  switch (build.kind) {
    case "instance":
      return build.core;
    case "core":
      return build.release;
    default:
      return shortRevision(build.revision);
  }
}

/**
 * The build as one line of plain text, for a problem report. English in every
 * locale: it is pasted into the core's issue tracker.
 */
export function buildReport(build: BuildRelease): string {
  switch (build.kind) {
    case "instance": {
      const instance = build.instance
        ? `instance ${build.instance}`
        : "instance build";
      const commit = build.revision
        ? ` (instance commit ${build.revision})`
        : "";
      return `Margince core ${build.core}, ${instance}${commit}`;
    }
    case "core":
      return `Margince ${build.release}${commitOf(build.revision)}`;
    default:
      return `Margince development build${commitOf(build.revision)}`;
  }
}

function commitOf(revision: string | undefined): string {
  return revision ? ` (commit ${revision})` : "";
}
