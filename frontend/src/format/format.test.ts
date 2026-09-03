import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  formatDate,
  formatDateTime,
  formatDayMonth,
  formatDuration,
  formatMoney,
  formatMoneyOrAbsent,
  formatNumber,
  formatSignedMoney,
  formatTimeOfDay,
  formatUsdPerMTok,
  identifierNumber,
  isRenderableZone,
  MONEY_ABSENT,
  ordinalNumber,
} from "./format";

// B-EP09.17/18/19 acceptance: locale changes the RENDERING of the same stored
// value and never the value; de-DE renders decimal-comma / dot-thousands;
// zones are IANA-only; durations are absolute; FX display never computes.

describe("money formatting (B-EP09.17)", () => {
  it("renders the same stored minor units differently per locale, value unchanged", () => {
    const stored = 123_456; // minor units, exactly as the API returns them
    const de = formatMoney(stored, "EUR", "de");
    const en = formatMoney(stored, "EUR", "en");
    expect(de).toBe("1.234,56\u00a0€");
    expect(en).toBe("€1,234.56");
    // formatting is a pure read: re-rendering the same stored value is stable,
    // and the two locales render the one value differently.
    expect(formatMoney(stored, "EUR", "de")).toBe(de);
    expect(de).not.toBe(en);
  });

  it("respects the currency's minor-unit scale", () => {
    // JPY has zero minor digits — 1234 minor units is ¥1,234, not ¥12.34
    expect(formatMoney(1234, "JPY", "en")).toContain("1,234");
  });

  // The SIGNED money formatter takes the same minor-unit scale. It is a second
  // function doing the conversion, so a regression here is invisible to every
  // formatMoney case above — and a yen delta rendered with two decimals is
  // wrong by a factor of a hundred.
  it("respects the currency's minor-unit scale for a change too", () => {
    // EXACT strings, not `toContain`. The amount is converted by
    // `toMajorUnits`, which reads the currency itself, so a wrong digit count
    // does not move the figure — it appends decimals a currency with no
    // subunit does not have. "+¥1,234.00" contains "1,234" and is still wrong,
    // which is how a looser assertion passed over exactly this defect.
    expect(formatSignedMoney(1234, "JPY", "en")).toBe("+JP¥1,234");
    expect(formatSignedMoney(1_284_000, "VND", "vi")).toBe("+1.284.000\u00a0₫");
  });

  // The sign always shows: a bare figure beside last week's leaves the reader
  // to work out which way it moved, and half of them will guess.
  it("always draws the direction a money change moved", () => {
    expect(formatSignedMoney(250000, "EUR", "en")).toContain("+");
    expect(formatSignedMoney(-250000, "EUR", "en")).toMatch(/[-−]/);
  });

  // A week that matched the one before is a real answer. "+0" dresses it as
  // growth, which is a small lie repeated weekly.
  it("marks an exactly level change with ± rather than +", () => {
    const level = formatSignedMoney(0, "EUR", "en");
    expect(level).toContain("±");
    expect(level).not.toContain("+");
  });

  // VND is a zero-decimal currency: formatMoney divides by
  // 10 ** maximumFractionDigits, which resolves to 1 here. Nothing pinned this,
  // and a regression would inflate every Vietnamese money figure 100×.
  it("renders a zero-decimal currency without shifting the decimal point", () => {
    expect(formatMoney(1_284_000, "VND", "vi")).toBe("1.284.000\u00a0₫");
  });

  it("still scales two-decimal currencies under vi", () => {
    expect(formatMoney(128_400, "EUR", "vi")).toBe("1.284,00\u00a0€");
  });
});

// Both halves of a money value are required, and neither absence has a safe
// default: an invented currency is indistinguishable from a real one, and an
// invented zero is a figure the server never sent.
describe("money with a half missing (data-semantics §1)", () => {
  it("renders a dash when the currency is absent, never a guessed one", () => {
    expect(formatMoneyOrAbsent(123_456, null, "en")).toBe(MONEY_ABSENT);
    expect(formatMoneyOrAbsent(123_456, undefined, "en")).toBe(MONEY_ABSENT);
    // The empty string is the shape that reached Intl and threw mid-render.
    expect(formatMoneyOrAbsent(123_456, "", "en")).toBe(MONEY_ABSENT);
  });

  it("renders a dash when the amount is absent, never a zero", () => {
    expect(formatMoneyOrAbsent(null, "EUR", "en")).toBe(MONEY_ABSENT);
    expect(formatMoneyOrAbsent(undefined, "EUR", "en")).toBe(MONEY_ABSENT);
    expect(formatMoneyOrAbsent(null, "EUR", "en")).not.toContain("0");
  });

  // A real zero is a figure the server DID send, and it is not the same claim
  // as an absent one.
  it("renders a stored zero as money", () => {
    expect(formatMoneyOrAbsent(0, "EUR", "en")).toBe("€0.00");
  });

  it("formats exactly as formatMoney once both halves are present", () => {
    expect(formatMoneyOrAbsent(123_456, "EUR", "de")).toBe(
      formatMoney(123_456, "EUR", "de"),
    );
  });
});

describe("date/time formatting (B-EP09.17/19)", () => {
  const instant = "2026-06-04T21:30:00Z";

  it("renders de-DE as dd.mm.yyyy", () => {
    expect(formatDate(instant, "de", "Europe/Berlin")).toBe("04.06.2026");
  });

  it("renders the same UTC instant with the correct zone per purpose", () => {
    // 21:30Z on 4 June is already 5 June in Auckland: the personal-deadline
    // zone and the workspace-reporting zone disagree on the calendar day.
    const userZone = formatDate(instant, "de", "Pacific/Auckland");
    const workspaceZone = formatDate(instant, "de", "Europe/Berlin");
    expect(userZone).toBe("05.06.2026");
    expect(workspaceZone).toBe("04.06.2026");
  });

  it("formats vi dates day-first", () => {
    expect(formatDate("2026-06-24T10:00:00Z", "vi", "Asia/Ho_Chi_Minh")).toBe(
      "24/06/2026",
    );
  });

  it("rejects fixed-offset zones — IANA names only (AC-DS-TZ4)", () => {
    expect(() => formatDate(instant, "de", "+01:00")).toThrow(/IANA/);
    expect(() => formatDateTime(instant, "de", "Etc/GMT-1")).toThrow(/IANA/);
    expect(() => formatDate(instant, "de", "GMT+1")).toThrow(/IANA/);
  });

  it("renders a day and month without a year, in the reader's own locale", () => {
    // Four screens carried a private copy of this rendering, every one of them
    // passing `undefined` — so the same person record printed its dates in the
    // browser's guessed locale on four surfaces. The locale is the whole point
    // of the assertion: de and en put the day and the month in different
    // orders, and a copy that took no locale would produce one of them for
    // every reader.
    expect(formatDayMonth(instant, "de", "Europe/Berlin")).toBe("4. Juni");
    expect(formatDayMonth(instant, "en", "Europe/Berlin")).toBe("4 Jun");
    // And it is genuinely the year-less sibling of formatDateAbbrev.
    expect(formatDayMonth(instant, "en", "Europe/Berlin")).not.toMatch(/2026/);
  });

  it("carries the zone into a day-month rendering too", () => {
    // 21:30Z on 4 June is already 5 June in Auckland. A day-month rendering
    // that dropped the zone would name the wrong day for half the world while
    // looking correct to whoever wrote it.
    expect(formatDayMonth(instant, "en", "Pacific/Auckland")).toBe("5 Jun");
  });

  it("renders a time of day in the zone it is given", () => {
    expect(formatTimeOfDay(instant, "en", "Europe/Berlin")).toBe("23:30");
    expect(formatTimeOfDay(instant, "en", "Pacific/Auckland")).toBe("09:30");
  });

  it("refuses a fixed offset from the day-month and time renderings as well", () => {
    // The rule is the module's, not one function's: a renderer added without
    // the assertion would accept an offset that freezes the DST rules of the
    // day it was picked.
    expect(() => formatDayMonth(instant, "de", "+01:00")).toThrow(/IANA/);
    expect(() => formatTimeOfDay(instant, "de", "Etc/GMT-1")).toThrow(/IANA/);
  });

  it("answers whether a zone renders, without throwing, and agrees with the formatters", () => {
    // A page rendering a LIST cannot let one row's zone take the page down, so
    // it asks first. Asked any other way the two answers come apart: probing
    // Intl alone learns only that the name RESOLVES, which every one of these
    // fixed offsets does — and a caller that trusted that probe then threw
    // inside the formatter one line later.
    for (const rejected of ["+01:00", "Etc/GMT-1", "Etc/GMT+5", "GMT"]) {
      expect(isRenderableZone(rejected)).toBe(false);
      // The half that makes the predicate worth having: Intl itself accepts
      // every one of these, so a probe built on Intl would have said yes.
      expect(() =>
        new Intl.DateTimeFormat("en-US", { timeZone: rejected }).format(),
      ).not.toThrow();
      expect(() => formatDateTime(instant, "en", rejected)).toThrow(/IANA/);
    }
    for (const accepted of ["Europe/Berlin", "Pacific/Auckland", "UTC"]) {
      expect(isRenderableZone(accepted)).toBe(true);
      expect(() => formatDateTime(instant, "en", accepted)).not.toThrow();
    }
    // A name no runtime knows is refused too, and still without throwing.
    expect(isRenderableZone("Not/AZone")).toBe(false);
  });

  it("renders idle spans as absolute durations, not calendar diffs", () => {
    expect(formatDuration(62 * 86_400_000, "en")).toMatch(/62/);
    expect(formatDuration(5 * 3_600_000, "en")).toMatch(/5/);
  });
});

describe("FX display discipline (B-EP09.18)", () => {
  const source = readFileSync(
    join(dirname(fileURLToPath(import.meta.url)), "format.ts"),
    "utf8",
  );
  const explainSource = readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      "..",
      "design-system",
      "explain.tsx",
    ),
    "utf8",
  );

  it("never issues a live FX call at render time", () => {
    for (const text of [source, explainSource]) {
      expect(text).not.toMatch(/fetch\s*\(|XMLHttpRequest|axios/);
    }
  });

  it("never multiplies native amounts by rates (consumes the IR base_value)", () => {
    // the lineage row fields exist for display; no arithmetic combines them
    expect(explainSource).not.toMatch(/nativeAmountMinor\s*\*|rate\s*\*/);
  });
});

// The ten codes where CLDR and ISO 4217 disagree are the ones that prove
// display and storage share a scale. A stored IQD 1234 is 1.234 dinars to the
// server, and Intl's own count would have rendered it as 1,234.
describe("display scales by the same table the server stores with", () => {
  it.each([
    ["IQD", 1234, "1.234"],
    ["MGA", 1234, "12.34"],
    ["IRR", 1234, "12.34"],
    ["VND", 18_000_000, "18,000,000"],
    ["EUR", 12_345, "123.45"],
  ])("%s %i renders the figure %s", (currency, minor, figure) => {
    // Only the digits are asserted: symbol placement and grouping are Intl's
    // to decide and are not what this pins.
    const rendered = formatMoney(minor, currency, "en").replace(/[^\d.,]/g, "");
    expect(rendered).toBe(figure);
  });
});

describe("a number that names or places is not a magnitude", () => {
  // The pair exists so a call site can SAY which of the two it meant, and the
  // claim is that they part company exactly where grouping starts. Four digits
  // is where de-DE first groups, so it is the only width that can show it.
  it("groups a magnitude in the reader's own notation", () => {
    expect(formatNumber(1234, "de")).toBe("1.234");
    expect(formatNumber(1234, "en")).toBe("1,234");
  });

  it("leaves a name and a position ungrouped, in every locale", () => {
    expect(identifierNumber(1234)).toBe("1234");
    expect(ordinalNumber(1234)).toBe("1234");
  });

  // Not a tautology about `String`: it is the claim that makes the ruling worth
  // spelling as a call. Revision 1234 rendered through the formatter beside it
  // reads as a quantity of revisions, and "1.204" is a different revision from
  // "1204" to anybody typing it into a search box.
  it("parts company with the formatter at four digits", () => {
    expect(identifierNumber(1234)).not.toBe(formatNumber(1234, "de"));
    expect(identifierNumber(1234)).not.toBe(formatNumber(1234, "en"));
    // And agrees with it below that width, which is why three of these sites
    // looked correct for as long as the demo data stayed small.
    expect(identifierNumber(999)).toBe(formatNumber(999, "de"));
  });
});

describe("formatUsdPerMTok", () => {
  // The sheet's own unit: USD per million tokens, as a decimal string. Always
  // USD — the contract says so, and the µUSD integers behind it carry no
  // currency of their own.
  it("reads the sheet's decimal string as USD", () => {
    // `US$`, not `$`: unconfigured English here is en-GB (A100), where the
    // dollar sign alone would not say WHOSE dollar.
    expect(formatUsdPerMTok("3", "en")).toBe("US$3.00");
    expect(formatUsdPerMTok("15", "en")).toBe("US$15.00");
  });

  it("renders the figure in the reader's conventions", () => {
    expect(formatUsdPerMTok("1234.5", "de")).toBe("1.234,50\u00a0$");
  });

  // The trap this exists to avoid: a real price shown as $0.00 reads as free,
  // and free is a claim this product is careful never to make by accident.
  it("never rounds a real price down to nothing", () => {
    expect(formatUsdPerMTok("0.01", "en")).toBe("US$0.01");
    expect(formatUsdPerMTok("0.005", "en")).toBe("US$0.005");
    expect(formatUsdPerMTok("0.0001", "en")).toBe("US$0.0001");
  });

  // A local model really does cost nothing, and saying so is the point of its
  // explicit zero row — an honest 0, never "no data".
  it("says zero for a price that is genuinely zero", () => {
    expect(formatUsdPerMTok("0", "en")).toBe("US$0.00");
  });
});
