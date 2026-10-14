// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { choicesOf, NO_EDITS } from "./importdraft";
import type { WizardFile } from "./importflow";
import type { ImportRun } from "./importtypes";

const file: WizardFile = {
  file: "contacts.csv",
  rowsProfiled: 3,
  columns: [],
  detected: "contact",
  suggested: { contact: { Name: "display_name" }, company: { Name: "name" } },
};

function runWith(object: string, mapping?: Record<string, string>): ImportRun {
  return {
    id: "r1",
    parts: [{ file: "contacts.csv", object, row_count: 0, mapping }],
  } as unknown as ImportRun;
}

describe("choicesOf saved mapping", () => {
  it("restores the mapping the run saved over the suggestion", () => {
    const run = runWith("contact", { Mail: "email" });
    const c = choicesOf([file], false, run, NO_EDITS, "");
    expect(c.mapping["contacts.csv"]).toEqual({ Mail: "email" });
  });

  it("falls back to the suggestion when the run saved none", () => {
    const c = choicesOf([file], false, runWith("contact"), NO_EDITS, "");
    expect(c.mapping["contacts.csv"]).toEqual({ Name: "display_name" });
  });

  it("lets a fresh edit win over the saved mapping", () => {
    const edits = { ...NO_EDITS, mapping: { "contacts.csv": { X: "phone" } } };
    const c = choicesOf(
      [file],
      false,
      runWith("contact", { A: "b" }),
      edits,
      "",
    );
    expect(c.mapping["contacts.csv"]).toEqual({ X: "phone" });
  });

  it("ignores a saved mapping once the file is retyped", () => {
    const edits = {
      ...NO_EDITS,
      objects: { "contacts.csv": "company" as const },
    };
    const c = choicesOf(
      [file],
      false,
      runWith("contact", { A: "b" }),
      edits,
      "",
    );
    expect(c.mapping["contacts.csv"]).toEqual({ Name: "name" });
  });
});
