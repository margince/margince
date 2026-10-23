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
  mode: "create" | "edit",
) {
  return projectOwnerField(t, picker, { key: "owner_id", mode });
}

describe("projectOwnerField", () => {
  // A create that names nobody is owned by its creator. So the reader is the
  // blank value, which is also the default.
  it("offers every colleague at birth with the creator first as the blank", () => {
    const [owner] = field({ owners, me: "u-me" }, "create");
    expect(owner?.required).toBeFalsy();
    expect(owner?.options).toEqual([
      { value: "", label: "Jane Doe" },
      { value: "u-other", label: "Omar Haddad" },
      { value: "u-third", label: "Lena Brandt" },
    ]);
  });

  it("names the creator as Me while the roster has not answered", () => {
    const [owner] = field({ owners: [], me: "u-me" }, "create");
    expect(owner?.options).toEqual([{ value: "", label: "<project.ownerMe>" }]);
  });

  it("offers an owned project's edit every colleague, then Unassigned, and never requires one", () => {
    const [owner] = field({ owners, me: "u-me" }, "edit");
    expect(owner?.required).toBeFalsy();
    expect(owner?.options).toEqual([
      { value: "u-me", label: "Jane Doe" },
      { value: "u-other", label: "Omar Haddad" },
      { value: "u-third", label: "Lena Brandt" },
      { value: "", label: "<list.unowned>" },
    ]);
  });

  it("offers Unassigned on an edit of a project nobody owns", () => {
    const [owner] = field({ owners, me: "u-me" }, "edit");
    expect(owner?.required).toBeFalsy();
    expect(owner?.options?.at(-1)).toEqual({
      value: "",
      label: "<list.unowned>",
    });
  });

  it("says so beneath the select when the colleague list is incomplete", () => {
    const [owner] = field(
      { owners: [], me: "u-me", hint: "Colleagues did not load." },
      "create",
    );
    expect(owner?.hint).toBe("Colleagues did not load.");
  });
});
