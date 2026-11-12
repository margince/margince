import { expect, it } from "vitest";
import { translate, translatePlural } from "../i18n";
import type { TrialStanding } from "../lib/license-posture";
import { trialPillText } from "./trialpill";

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);
const plural = (
  base: Parameters<typeof translatePlural>[1],
  count: number,
  params?: Record<string, string>,
) => translatePlural("en", base, count, params);

function standing(daysLeft: number, closing: boolean): TrialStanding {
  return { endsAt: "2026-11-16T09:30:00Z", daysLeft, ended: false, closing };
}

it("says the trial's state in the words of each stage", () => {
  expect(trialPillText(standing(10, false), t, plural)).toBe(
    "Trial · 10 days left",
  );
  expect(trialPillText(standing(6, true), t, plural)).toBe(
    "6 days left of your trial",
  );
  expect(trialPillText(standing(1, true), t, plural)).toBe(
    "Last day of your trial",
  );
  expect(trialPillText({ ...standing(0, true), ended: true }, t, plural)).toBe(
    "Trial ended · Contact Margince",
  );
});
