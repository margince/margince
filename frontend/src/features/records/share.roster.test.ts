// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { RosterSubject } from "../../components/share.parts";
import { pickedToGrant } from "./share.roster";

const priya: RosterSubject = {
  id: "u-1",
  name: "Priya Shah",
  note: "",
  kind: "user",
};
const desk: RosterSubject = {
  id: "t-1",
  name: "Deal Desk",
  note: "",
  kind: "team",
};

describe("the subject a Grant press sends", () => {
  it("is dropped once the access answer names the picked colleague as owner", () => {
    expect(pickedToGrant(priya, "u-1")).toBeNull();
  });

  it("is no colleague while the owner is not known yet", () => {
    expect(pickedToGrant(priya, undefined)).toBeNull();
  });

  it("keeps a team, and a colleague on a record with no owner", () => {
    expect(pickedToGrant(desk, undefined)).toBe(desk);
    expect(pickedToGrant(priya, null)).toBe(priya);
  });
});
