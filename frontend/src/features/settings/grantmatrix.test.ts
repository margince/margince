// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { nextGrant } from "./grantmatrix";

const NONE = { read: false, create: false, update: false, delete: false };
const ALL = { read: true, create: true, update: true, delete: true };

describe("nextGrant", () => {
  it("turns the writes off with Read", () => {
    expect(nextGrant(ALL, "read", false)).toEqual(NONE);
  });

  it("turns Read on with any write verb", () => {
    expect(nextGrant(NONE, "delete", true)).toEqual({
      ...NONE,
      read: true,
      delete: true,
    });
  });

  it("repairs a stored write without Read when another switch moves", () => {
    expect(nextGrant({ ...NONE, create: true }, "update", false)).toEqual({
      ...NONE,
      read: true,
      create: true,
    });
  });

  it("leaves the writes alone for an object that offers no Read", () => {
    expect(nextGrant(NONE, "delete", true, ["delete"])).toEqual({
      ...NONE,
      delete: true,
    });
  });
});
