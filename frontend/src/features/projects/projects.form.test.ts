// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  mapProjectCreate,
  mapProjectUpdate,
  projectFields,
} from "./projects.form";

// The project form's pure half: the key rule the contract states as a
// pattern, and the two bodies the dialogs build from their values.

// The key is minted by the server and is not a field either dialog offers: a
// caller-chosen key is a subject-line matcher a caller can get wrong, so the
// form must not grow one back.
describe("the project form offers no key field", () => {
  const t = (key: string) => `<${key}>`;
  it("leaves the key out of both dialogs", () => {
    for (const mode of ["create", "edit"] as const) {
      const fields = projectFields(t, {
        owner: { owners: [], me: "u1" },
        mode,
      });
      expect(fields.map((field) => field.key)).not.toContain("key");
    }
  });
});

describe("projectFields", () => {
  const t = (key: string) => `<${key}>`;

  it("asks for the company at birth and never on edit", () => {
    const create = projectFields(t, {
      owner: { owners: [], me: "u-me" },
      mode: "create",
    });
    const edit = projectFields(t, {
      owner: { owners: [], me: "u-me" },
      mode: "edit",
    });
    expect(create.map((field) => field.key)).toEqual([
      "name",
      "company_id",
      "owner_id",
      "description",
      "target_end_date",
    ]);
    expect(edit.map((field) => field.key)).not.toContain("company_id");
    expect(edit.map((field) => field.key)).toContain("owner_id");
  });
});

describe("mapProjectCreate / mapProjectUpdate", () => {
  it("builds the birth body with blanks as null and a manual source", () => {
    expect(
      mapProjectCreate({
        name: " Rollout ",
        // A key the caller somehow supplied is DROPPED, not forwarded: the
        // server mints the key and a body carrying one would be sending a
        // matcher nobody asked for.
        key: "CALLER-CHOSE",
        company_id: "o-1",
        owner_id: "",
        description: "",
        target_end_date: "2026-12-31",
      }),
    ).toEqual({
      name: "Rollout",
      company_id: "o-1",
      owner_id: null,
      description: null,
      target_end_date: "2026-12-31",
      source: "manual",
    });
  });

  it("clears a blanked scalar on update but never sends an empty name", () => {
    const patch = mapProjectUpdate({
      name: "",
      key: "ACME",
      owner_id: "",
      description: "Phase two",
      target_end_date: "",
    });
    expect(patch).toEqual({
      name: undefined,
      owner_id: null,
      description: "Phase two",
      target_end_date: null,
    });
    expect("phase" in patch).toBe(false);
  });
});
