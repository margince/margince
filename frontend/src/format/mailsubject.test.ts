// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { withoutSubjectPrefixes } from "./mailsubject";

describe("withoutSubjectPrefixes", () => {
  it("removes every stacked reply and forward marker in front", () => {
    expect(withoutSubjectPrefixes("AW: WG: Budget")).toBe("Budget");
    expect(withoutSubjectPrefixes(" FW: re:AW: Budget")).toBe("Budget");
  });

  it("keeps a marker that is not in front", () => {
    expect(withoutSubjectPrefixes("Budget Re: Q3")).toBe("Budget Re: Q3");
  });
});
