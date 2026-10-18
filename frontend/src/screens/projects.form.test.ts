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
        companies: [],
        owners: [],
        me: "u1",
        currentOwner: null,
        mode,
      });
      expect(fields.map((field) => field.key)).not.toContain("key");
    }
  });
});

describe("projectFields", () => {
  const t = (key: string) => `<${key}>`;
  const owners = [
    { value: "u-other", label: "Omar Haddad" },
    { value: "u-me", label: "Jane Doe" },
    { value: "u-third", label: "Lena Brandt" },
  ];

  it("asks for the company at birth and never on edit", () => {
    const create = projectFields(t, {
      companies: [{ id: "o-1", display_name: "Brandt" }],
      owners,
      me: "u-me",
      currentOwner: null,
      mode: "create",
    });
    const edit = projectFields(t, {
      companies: [],
      owners,
      me: "u-me",
      currentOwner: "u-other",
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

  // A required select opens on its first option, so the creator heading the
  // list is what makes the owner default to them.
  it("offers every colleague at birth with the creator first and required", () => {
    const create = projectFields(t, {
      companies: [],
      owners,
      me: "u-me",
      currentOwner: null,
      mode: "create",
    });
    const owner = create.find((field) => field.key === "owner_id");
    expect(owner?.required).toBe(true);
    expect(owner?.options?.map((option) => option.value)).toEqual([
      "u-me",
      "u-other",
      "u-third",
    ]);
  });

  it("keeps the creator first while the roster has not answered", () => {
    const create = projectFields(t, {
      companies: [],
      owners: [],
      me: "u-me",
      currentOwner: null,
      mode: "create",
    });
    const owner = create.find((field) => field.key === "owner_id");
    expect(owner?.options).toEqual([
      { value: "u-me", label: "<project.ownerMe>" },
    ]);
  });

  it("leaves the owner to the server when the creator is not known", () => {
    const create = projectFields(t, {
      companies: [],
      owners,
      me: "",
      currentOwner: null,
      mode: "create",
    });
    expect(create.map((field) => field.key)).not.toContain("owner_id");
  });

  // A blank owner is sent as null, which the update reads as "not supplied".
  // So a blank choice on an owned project would save nothing.
  it("lets an edit pick any colleague and offers no blank for an owned project", () => {
    const edit = projectFields(t, {
      companies: [],
      owners,
      me: "u-me",
      currentOwner: "u-other",
      mode: "edit",
    });
    const owner = edit.find((field) => field.key === "owner_id");
    expect(owner?.required).toBe(true);
    expect(owner?.options?.map((option) => option.value)).toEqual([
      "u-me",
      "u-other",
      "u-third",
    ]);
  });

  it("keeps the blank on an edit of a project nobody owns", () => {
    const edit = projectFields(t, {
      companies: [],
      owners,
      me: "u-me",
      currentOwner: null,
      mode: "edit",
    });
    expect(edit.find((field) => field.key === "owner_id")?.required).toBe(
      false,
    );
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
