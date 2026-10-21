// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { projectOwnerField } from "./projectownerfield";

const t = (key: string) => `<${key}>`;
const owners = [
  { value: "u-other", label: "Omar Haddad" },
  { value: "u-me", label: "Jane Doe" },
  { value: "u-third", label: "Lena Brandt" },
];

function field(
  picker: Parameters<typeof projectOwnerField>[1],
  currentOwner: string | null,
  mode: "create" | "edit",
) {
  return projectOwnerField(t, picker, { key: "owner_id", currentOwner, mode });
}

describe("projectOwnerField", () => {
  // A create that names nobody is owned by its creator. So the reader is the
  // blank value, which is also the default.
  it("offers every colleague at birth with the creator first as the blank", () => {
    const [owner] = field({ owners, me: "u-me" }, null, "create");
    expect(owner?.required).toBe(false);
    expect(owner?.options).toEqual([
      { value: "", label: "Jane Doe" },
      { value: "u-other", label: "Omar Haddad" },
      { value: "u-third", label: "Lena Brandt" },
    ]);
  });

  it("names the creator as Me while the roster has not answered", () => {
    const [owner] = field({ owners: [], me: "u-me" }, null, "create");
    expect(owner?.options).toEqual([{ value: "", label: "<project.ownerMe>" }]);
  });

  // A blank owner is sent as null, which the update reads as "not supplied".
  // So a blank choice on an owned project would save nothing.
  it("lets an edit pick any colleague and offers no blank for an owned project", () => {
    const [owner] = field({ owners, me: "u-me" }, "u-other", "edit");
    expect(owner?.required).toBe(true);
    expect(owner?.options?.map((option) => option.value)).toEqual([
      "u-me",
      "u-other",
      "u-third",
    ]);
  });

  it("keeps the blank on an edit of a project nobody owns", () => {
    const [owner] = field({ owners, me: "u-me" }, null, "edit");
    expect(owner?.required).toBe(false);
  });

  it("says so beneath the select when the colleague list is incomplete", () => {
    const [owner] = field(
      { owners: [], me: "u-me", hint: "Colleagues did not load." },
      null,
      "create",
    );
    expect(owner?.hint).toBe("Colleagues did not load.");
  });
});
