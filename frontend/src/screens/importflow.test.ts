// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { stepOfRun, type WizardStep } from "./importflow";
import type { ImportRun } from "./importtypes";

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
    for (const [status, step] of Object.entries(STEP_OF_STATUS)) {
      expect(stepOfRun(status as ImportRun["status"], "map")).toBe(step);
    }
    expect(stepOfRun("pending", "files")).toBe("files");
    expect(stepOfRun(undefined, "map")).toBe("test");
  });
});
