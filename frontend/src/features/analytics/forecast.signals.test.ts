import { describe, expect, it } from "vitest";
import { translate, translatePlural } from "../../i18n";
import { type RiskSignal, signalSentence } from "./forecast.signals";

const STAGE = "018f0000-0000-7000-8000-0000000000aa";

function sentence(signal: RiskSignal, locale: "en" | "de" | "vi" = "en") {
  return signalSentence(
    signal,
    (key, params) => translate(locale, key, params),
    (base, count, params) => translatePlural(locale, base, count, params),
    (id) => (id === STAGE ? "Proposal" : undefined),
    (value) => String(value),
  );
}

describe("signalSentence", () => {
  it.each([
    [
      { code: "stuck", days: 21, stage_id: STAGE },
      "Stalled 21 days in Proposal",
    ],
    [{ code: "stuck", days: 1, stage_id: STAGE }, "Stalled 1 day in Proposal"],
    [{ code: "stuck", days: 9 }, "Stalled 9 days in this stage"],
    [{ code: "close_pushed", count: 3 }, "Close date moved 3 times"],
    [
      { code: "close_pushed", count: 3, close_passed: true },
      "Close date passed",
    ],
    [{ code: "quiet", days: 34 }, "No reply in 34 days"],
    [{ code: "no_next_step" }, "No next step booked"],
    [
      { code: "no_next_step", closes_in_days: 0 },
      "Closes today, no meeting booked",
    ],
    [
      { code: "no_next_step", closes_in_days: 1 },
      "Closes tomorrow, no meeting booked",
    ],
    [
      { code: "no_next_step", closes_in_days: 5 },
      "Closes in 5 days, no meeting booked",
    ],
  ] satisfies [RiskSignal, string][])("words %j as %s", (signal, want) => {
    expect(sentence(signal)).toBe(want);
  });

  it("words the numbers in German and Vietnamese too", () => {
    expect(sentence({ code: "stuck", days: 21, stage_id: STAGE }, "de")).toBe(
      "Seit 21 Tagen in Proposal",
    );
    expect(sentence({ code: "quiet", days: 1 }, "vi")).toBe(
      "Không có phản hồi trong 1 ngày",
    );
  });
});
