/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { RecordZoneProvider } from "../app/recordzone";
import { LocaleProvider } from "../i18n";
import { useHealthReason } from "./companylookups";

// The server rates relationship and commercial health and names a reason code
// beside its English sentence. The reader sees the code in their language, and
// the English only when there is no code to say.

function wrapper(locale: "en" | "de" | "vi", zone = "UTC") {
  return ({ children }: { children: ReactNode }) => (
    <LocaleProvider initial={locale}>
      <RecordZoneProvider zone={zone}>{children}</RecordZoneProvider>
    </LocaleProvider>
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

  // The day is the record zone's, not UTC's: 17:30Z is already the next
  // morning in Ho Chi Minh City.
  it("names a booked meeting's day in the record's zone", () => {
    const en = renderHook(() => useHealthReason(), {
      wrapper: wrapper("en", "Asia/Ho_Chi_Minh"),
    });
    const said = en.result.current({
      reason: "A meeting with them is booked.",
      reason_code: "meeting_booked",
      reason_params: { at: "2026-10-08T17:30:00Z" },
    });
    expect(said).toMatch(/\b9\b/);
    expect(said).not.toMatch(/\b8\b/);
    expect(said).not.toContain("{at}");
  });

  // A code whose sentence needs a value the server did not send is said in
  // the server's own words, never as "{days}" or a made-up zero.
  it("falls back to the sentence when a value the code needs is missing", () => {
    const en = renderHook(() => useHealthReason(), { wrapper: wrapper("en") });
    expect(en.result.current({ reason: english, reason_code: "quiet" })).toBe(
      english,
    );
    expect(
      en.result.current({
        reason: "2 of 5 open deals have stalled.",
        reason_code: "deals_some_stalled",
        reason_params: { count: 2 },
      }),
    ).toBe("2 of 5 open deals have stalled.");
  });

  // Payment is rated on this side and carries no code; an older server sends
  // none either. Both still read, in the words they came with.
  it("falls back to the sentence when there is no code", () => {
    const vi = renderHook(() => useHealthReason(), { wrapper: wrapper("vi") });
    expect(vi.result.current({ reason: english })).toBe(english);
  });
});
