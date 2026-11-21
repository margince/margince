// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { fileListKeys } from "./attachmentkeys";

describe("fileListKeys", () => {
  it("refreshes the project page after an upload to the project", () => {
    expect(fileListKeys({ entityType: "project", entityId: "pr-1" })).toEqual([
      ["project", "pr-1"],
    ]);
  });

  it("refreshes every project page after a file lands on an activity", () => {
    expect(
      fileListKeys({ entityType: "activity", entityId: "a-1" }),
    ).toContainEqual(["project"]);
  });
});
