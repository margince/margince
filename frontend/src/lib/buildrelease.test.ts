// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  type BuildRelease,
  buildRelease,
  buildReport,
  reportedRelease,
} from "./buildrelease";

// The stamped constants are compiled in and empty in every local build.
// Getters over a hoisted value let each case stamp the ones it needs.
const bundle = vi.hoisted(() => ({ release: "", core: "", revision: "" }));
vi.mock("./release", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./release")>();
  return {
    ...actual,
    get SPA_RELEASE() {
      return bundle.release;
    },
    get CORE_RELEASE() {
      return bundle.core;
    },
    get BUILD_REVISION() {
      return bundle.revision;
    },
  };
});

afterEach(() => {
  bundle.release = "";
  bundle.core = "";
  bundle.revision = "";
});

const SHA = "0123456789abcdef0123456789abcdef01234567";
const CORE = "2026.2.1";

describe("buildRelease", () => {
  it("reads a stamped release as a core build", () => {
    bundle.release = CORE;
    bundle.revision = SHA;
    expect(buildRelease()).toEqual({
      kind: "core",
      release: CORE,
      revision: SHA,
    });
  });

  // "dev" is what a plain docker build stamps, so it is no core release.
  it('reads a core release of "dev" beside a stamped release as a core build', () => {
    bundle.release = CORE;
    bundle.core = "dev";
    expect(buildRelease()).toEqual({
      kind: "core",
      release: CORE,
      revision: undefined,
    });
  });

  it.each(["", "dev"])(
    "reads an instance build whose own release is unstamped (%j)",
    (release) => {
      bundle.release = release;
      bundle.core = CORE;
      expect(buildRelease()).toEqual({
        kind: "instance",
        core: CORE,
        instance: undefined,
        revision: undefined,
      });
    },
  );

  // Equal values are still two facts about two repositories.
  it("keeps an instance release equal to its core", () => {
    bundle.release = CORE;
    bundle.core = CORE;
    expect(buildRelease()).toMatchObject({
      kind: "instance",
      core: CORE,
      instance: CORE,
    });
  });

  it.each(["", "dev"])(
    "carries no revision when the stamp is %j",
    (revision) => {
      bundle.revision = revision;
      expect(buildRelease()).toEqual({
        kind: "unstamped",
        revision: undefined,
      });
    },
  );
});

describe("reportedRelease", () => {
  it.each<[BuildRelease, string | undefined]>([
    [{ kind: "core", release: CORE, revision: SHA }, CORE],
    [{ kind: "instance", core: CORE, instance: "2026.2.1-acme.3" }, CORE],
    [{ kind: "unstamped", revision: SHA }, "0123456"],
    [{ kind: "unstamped" }, undefined],
  ])("names the release the core tracker knows (%j)", (build, release) => {
    expect(reportedRelease(build)).toBe(release);
  });
});

describe("buildReport", () => {
  it.each<[string, BuildRelease, string]>([
    [
      "a core build",
      { kind: "core", release: CORE, revision: SHA },
      `Margince ${CORE} (commit ${SHA})`,
    ],
    [
      "a core build without a revision",
      { kind: "core", release: CORE },
      `Margince ${CORE}`,
    ],
    [
      "an instance build, whose commit is the instance's",
      {
        kind: "instance",
        core: CORE,
        instance: "2026.2.1-acme.3",
        revision: SHA,
      },
      `Margince core ${CORE}, instance 2026.2.1-acme.3 (instance commit ${SHA})`,
    ],
    [
      "an instance build without its own release",
      { kind: "instance", core: CORE, revision: SHA },
      `Margince core ${CORE}, instance build (instance commit ${SHA})`,
    ],
    [
      "an instance build without a revision",
      { kind: "instance", core: CORE },
      `Margince core ${CORE}, instance build`,
    ],
    [
      "an unstamped build",
      { kind: "unstamped", revision: SHA },
      `Margince development build (commit ${SHA})`,
    ],
    [
      "an unstamped build without a revision",
      { kind: "unstamped" },
      "Margince development build",
    ],
  ])("is one plain line for %s", (_, build, line) => {
    expect(buildReport(build)).toBe(line);
  });
});
