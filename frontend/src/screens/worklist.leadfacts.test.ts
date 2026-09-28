// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { type Translator, translate } from "../i18n";
import { dealFactsText } from "./worklist.copy";
import { leadFactsText } from "./worklist.leadfacts";
import type { WorklistItem } from "./worklist.queries";

const t: Translator = (key, params) => translate("en", key, params);
const base: WorklistItem = {
  id: "row",
  source: "lead_response",
  category: "leads",
  level: 4,
  consequence: "none",
  because: [],
  actions: ["open"],
};

it("gives an SDR contact context without inventing a response deadline", () => {
  const text = leadFactsText(
    {
      ...base,
      lead: {
        company_name: "North Logistics",
        status: "contacted",
        source: "Referral",
        last_activity_at: "2026-09-01T12:00:00Z",
        response_target_tracked: false,
      },
    },
    t,
    "en",
    "UTC",
  );
  expect(text).toContain("North Logistics");
  expect(text).toContain("Contacted");
  expect(text).toContain("Referral");
  expect(text).toContain("no response target set");
  expect(leadFactsText(base, t, "en", "UTC")).toBeNull();
});

it("keeps a provisional close on its calendar date in both hemispheres", () => {
  const item: WorklistItem = {
    ...base,
    source: "deal_at_risk",
    category: "deals_at_risk",
    deal: {
      expected_close_date: "2026-09-27",
      close_date_provisional: true,
      forecast_category: "omitted",
    },
  };
  const west = dealFactsText(item, t, "en", "America/Los_Angeles");
  const east = dealFactsText(item, t, "en", "Asia/Bangkok");
  expect(west).toEqual(east);
  expect(west).toContain("27/09/2026");
  expect(west).toContain("unconfirmed");
  expect(west).toContain("omitted from forecast");
});
