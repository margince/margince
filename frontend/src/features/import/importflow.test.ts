// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { stable } from "../../format/collate";
import { formatFixture } from "../../testing/import.fixtures";
import {
  SOURCE_NAME_MAX,
  sourceNameFits,
  stepOfRun,
  uploadObjectFor,
  type WizardStep,
} from "./importflow";
import { moving, wroteNothing } from "./importrun";
import type { ImportRun } from "./importtypes";

const STATUSES = [
  "pending",
  "validating",
  "awaiting_approval",
  "running",
  "failed",
  "complete",
  "undoing",
  "undone",
  "cancelled",
] as const satisfies readonly ImportRun["status"][];

// One row per status the contract names; a status it adds fails to compile
// here until the wizard is taught where that run stands.
const STEP_OF_STATUS = {
  pending: "map",
  validating: "test",
  awaiting_approval: "test",
  running: "import",
  failed: "import",
  complete: "result",
  undoing: "result",
  undone: "result",
  cancelled: "upload",
} as const satisfies Record<ImportRun["status"], WizardStep>;

describe("stepOfRun", () => {
  it("stepOfRun answers every status", () => {
    expect([...STATUSES].sort(stable)).toEqual(
      Object.keys(STEP_OF_STATUS).sort(stable),
    );
    for (const status of STATUSES) {
      expect(stepOfRun(status, "map")).toBe(STEP_OF_STATUS[status]);
    }
    expect(stepOfRun("pending", "files")).toBe("files");
    expect(stepOfRun(undefined, "map")).toBe("test");
  });

  it("keeps a run in a status this build does not know on the test step", () => {
    expect(stepOfRun("paused_later" as ImportRun["status"], "map")).toBe(
      "test",
    );
  });
});

describe("what a run's status says to the screen", () => {
  it("polls a run the server is still testing, writing or reversing", () => {
    expect(STATUSES.filter(moving)).toEqual([
      "validating",
      "running",
      "undoing",
    ]);
  });

  it("names a draft and a cancelled run as runs that wrote nothing", () => {
    expect(STATUSES.filter(wroteNothing)).toEqual(["pending", "cancelled"]);
  });
});

describe("the type a single CSV is uploaded as", () => {
  it("is the type a bundle file name gives, in any case", () => {
    expect(uploadObjectFor("Contacts.CSV", formatFixture)).toBe("contact");
  });

  it("is none for a name that names no type, nor for a ZIP", () => {
    expect(
      uploadObjectFor("clean-contacts.csv", formatFixture),
    ).toBeUndefined();
    expect(uploadObjectFor("crm.zip", formatFixture)).toBeUndefined();
  });

  it("is a preset type over the name", () => {
    expect(uploadObjectFor("leads.csv", formatFixture, "activity")).toBe(
      "activity",
    );
  });
});

describe("sourceNameFits", () => {
  it("takes letters A to Z, digits, space, dot, underscore and hyphen", () => {
    expect(sourceNameFits("")).toBe(true);
    expect(sourceNameFits("crm export_2026-11.v2")).toBe(true);
    expect(sourceNameFits("  padded  ")).toBe(true);
  });

  it("refuses any other character and a name over the limit", () => {
    for (const name of ["wiz ünïcödé!", "a/b", "tab\there", "née"]) {
      expect(sourceNameFits(name)).toBe(false);
    }
    expect(sourceNameFits("a".repeat(SOURCE_NAME_MAX))).toBe(true);
    expect(sourceNameFits("a".repeat(SOURCE_NAME_MAX + 1))).toBe(false);
  });
});
