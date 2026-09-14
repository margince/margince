/** @vitest-environment happy-dom */
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider, useLocale } from "../i18n";
import { formatDate, formatDateTime, formatTimeOfDay } from "./format";
import { setDateTimePreferences } from "./preferences";

afterEach(() => {
  cleanup();
  setDateTimePreferences({ dateFormat: "locale", timeFormat: "locale" });
});
const instant = "2026-09-13T18:05:00Z";

it.each([
  ["dmy", "13.09.2026"],
  ["mdy", "09/13/2026"],
  ["ymd", "2026-09-13"],
] as const)(
  "uses explicit %s dates in every interface language",
  (dateFormat, expected) => {
    setDateTimePreferences({ dateFormat, timeFormat: "24h" });
    for (const locale of ["en", "de", "vi"] as const) {
      expect(formatDate(instant, locale, "UTC")).toBe(expected);
      expect(formatDateTime(instant, locale, "UTC")).toBe(`${expected}, 18:05`);
    }
  },
);

it("changes notation without changing the record's timezone or midnight", () => {
  setDateTimePreferences({ dateFormat: "ymd", timeFormat: "24h" });
  expect(formatDateTime(instant, "en", "Asia/Bangkok")).toBe(
    "2026-09-14, 01:05",
  );
  expect(formatTimeOfDay("2026-09-13T00:05:00Z", "en", "UTC")).toBe("00:05");
  setDateTimePreferences({ dateFormat: "locale", timeFormat: "12h" });
  expect(formatTimeOfDay(instant, "en", "UTC")).toMatch(/6:05\s*pm/i);
});

it("updates mounted readers when installation preferences change", () => {
  function Reading() {
    const { locale } = useLocale();
    return <p>{formatDate(instant, locale, "UTC")}</p>;
  }
  render(
    <LocaleProvider>
      <Reading />
    </LocaleProvider>,
  );
  act(() => setDateTimePreferences({ dateFormat: "ymd", timeFormat: "24h" }));
  expect(screen.getByText("2026-09-13")).toBeTruthy();
  act(() => setDateTimePreferences({ dateFormat: "dmy", timeFormat: "24h" }));
  expect(screen.getByText("13.09.2026")).toBeTruthy();
});

it("keeps date-only records on their named calendar day in every notation", () => {
  for (const zone of [
    "America/New_York",
    "Europe/Berlin",
    "Pacific/Auckland",
  ]) {
    for (const dateFormat of ["locale", "dmy", "mdy", "ymd"] as const) {
      setDateTimePreferences({ dateFormat, timeFormat: "locale" });
      expect(formatDate("2026-09-30", "en", zone)).toBe(
        formatDate("2026-09-30T12:00:00Z", "en", "UTC"),
      );
    }
  }
});
