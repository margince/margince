// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { isForwardSubject, withoutSubjectPrefixes } from "./mailsubject";

describe("withoutSubjectPrefixes", () => {
  it("removes every stacked reply and forward marker in front", () => {
    expect(withoutSubjectPrefixes("AW: WG: Budget")).toBe("Budget");
    expect(withoutSubjectPrefixes(" FW: re:AW: Budget")).toBe("Budget");
  });

  it("keeps a marker that is not in front", () => {
    expect(withoutSubjectPrefixes("Budget Re: Q3")).toBe("Budget Re: Q3");
  });
});

describe("isForwardSubject", () => {
  it("reads the outermost prefix", () => {
    expect(isForwardSubject("WG: Angebot")).toBe(true);
    expect(isForwardSubject("  fwd:Angebot")).toBe(true);
    expect(isForwardSubject("\u00a0WG: Angebot")).toBe(true);
    expect(isForwardSubject("\u2003WG: Angebot")).toBe(false);
    expect(isForwardSubject("Fwd: Re: Angebot")).toBe(true);
    expect(isForwardSubject("Re: Fwd: Angebot")).toBe(false);
    expect(isForwardSubject("Forward planning")).toBe(false);
    expect(isForwardSubject("Fwdx: Angebot")).toBe(false);
    expect(isForwardSubject("")).toBe(false);
  });
});
