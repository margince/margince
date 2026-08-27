/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { LocaleProvider, usePlural, usePluralKey } from "./index";

// The two hooks bind the plural rule to the READER's locale, and that binding is
// the whole of what they add over the functions underneath. So these cases are
// about the locale coming from the provider rather than from the call: a helper
// that took the locale as an argument was how three surfaces ended up rendering
// English plurals to a German reader.

function wrapper(locale: "en" | "de" | "vi") {
  return ({ children }: { children: ReactNode }) => (
    <LocaleProvider initial={locale}>{children}</LocaleProvider>
  );
}

describe("usePlural", () => {
  it("renders the reader's own language and form", () => {
    const en = renderHook(() => usePlural(), { wrapper: wrapper("en") });
    expect(en.result.current("share.teamMembers", 1, { count: "1" })).toBe(
      "Team · 1 member",
    );
    expect(en.result.current("share.teamMembers", 4, { count: "4" })).toBe(
      "Team · 4 members",
    );

    const de = renderHook(() => usePlural(), { wrapper: wrapper("de") });
    expect(de.result.current("share.teamMembers", 1, { count: "1" })).toBe(
      "Team · 1 Mitglied",
    );
    expect(de.result.current("share.teamMembers", 3, { count: "3" })).toBe(
      "Team · 3 Mitglieder",
    );
  });

  it("gives Vietnamese one wording for both counts, because vi has one form", () => {
    const vi = renderHook(() => usePlural(), { wrapper: wrapper("vi") });
    expect(vi.result.current("share.teamMembers", 1, { count: "1" })).toBe(
      vi.result.current("share.teamMembers", 9, { count: "1" }),
    );
  });
});

describe("usePluralKey", () => {
  it("names the catalogue entry a count resolves to, in the reader's locale", () => {
    const en = renderHook(() => usePluralKey(), { wrapper: wrapper("en") });
    expect(en.result.current("ob.conv.review.requiredRemaining", 1)).toBe(
      "ob.conv.review.requiredRemaining_one",
    );
    expect(en.result.current("ob.conv.review.requiredRemaining", 2)).toBe(
      "ob.conv.review.requiredRemaining_other",
    );

    // vi has a single category, and it is `other` — so a caller carrying keys
    // gets the one arm that exists rather than a singular nothing reaches.
    const vi = renderHook(() => usePluralKey(), { wrapper: wrapper("vi") });
    expect(vi.result.current("ob.conv.review.requiredRemaining", 1)).toBe(
      "ob.conv.review.requiredRemaining_other",
    );
  });
});
