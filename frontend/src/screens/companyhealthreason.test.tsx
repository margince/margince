/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { useHealthReason } from "./companylookups";

// The server rates relationship and commercial health and names a reason code
// beside its English sentence. The reader sees the code in their language, and
// the English only when there is no code to say.

function wrapper(locale: "en" | "de" | "vi") {
  return ({ children }: { children: ReactNode }) => (
    <LocaleProvider initial={locale}>{children}</LocaleProvider>
  );
}

const english = "No reply and no meeting for 77 days.";

describe("useHealthReason", () => {
  it("says a coded reason in the reader's language, with its values", () => {
    const de = renderHook(() => useHealthReason(), { wrapper: wrapper("de") });
    expect(
      de.result.current({
        reason: english,
        reason_code: "quiet",
        reason_params: { days: 77 },
      }),
    ).toBe("Seit 77 Tagen keine Antwort und kein Treffen.");
  });

  it("counts the sentence by its number", () => {
    const en = renderHook(() => useHealthReason(), { wrapper: wrapper("en") });
    expect(
      en.result.current({
        reason: "Last met them 1 days ago.",
        reason_code: "last_met",
        reason_params: { days: 1 },
      }),
    ).toBe("Last met them 1 day ago.");
  });

  it("names the day a booked meeting is on", () => {
    const en = renderHook(() => useHealthReason(), { wrapper: wrapper("en") });
    const said = en.result.current({
      reason: "A meeting is booked for 8 October 2026.",
      reason_code: "meeting_booked",
      reason_params: { on: "2026-10-08" },
    });
    expect(said).toMatch(/^A meeting is booked for .*2026\.$/);
    expect(said).not.toContain("{on}");
  });

  // Payment is rated on this side and carries no code; an older server sends
  // none either. Both still read, in the words they came with.
  it("falls back to the sentence when there is no code", () => {
    const vi = renderHook(() => useHealthReason(), { wrapper: wrapper("vi") });
    expect(vi.result.current({ reason: english })).toBe(english);
  });
});
