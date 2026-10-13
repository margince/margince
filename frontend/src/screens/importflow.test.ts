// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { stepOfRun, type WizardStep } from "./importflow";
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
    expect([...STATUSES].sort()).toEqual(Object.keys(STEP_OF_STATUS).sort());
    for (const status of STATUSES) {
      expect(stepOfRun(status, "map")).toBe(STEP_OF_STATUS[status]);
    }
    expect(stepOfRun("pending", "files")).toBe("files");
    expect(stepOfRun(undefined, "map")).toBe("test");
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
