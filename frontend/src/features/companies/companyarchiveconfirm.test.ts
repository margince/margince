// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate, translatePlural } from "../../i18n";
import { company, company360 } from "../../testing/company.fixtures";
import type { Company360View } from "./companies";
import { companyArchiveConfirm } from "./companyarchiveconfirm";

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);
const plural = (
  base: Parameters<typeof translatePlural>[1],
  count: number,
  params?: Record<string, string>,
) => translatePlural("en", base, count, params);

function viewWith(contacts: number, openDeals: number): Company360View {
  return {
    ...company360,
    contacts: {
      data: [],
      page: { next_cursor: null, has_more: false, total: contacts },
    },
    state_strip: {
      account: { status: "customer", relationship_types: [] },
      commercial: {
        open_count: openDeals,
        stalled_count: 0,
        priced_count: openDeals,
        converted_count: 0,
        open_pipeline_minor_base: 990000,
        base_currency: "EUR",
      },
    },
  };
}

describe("companyArchiveConfirm", () => {
  it("counts the contacts and open deals and says what happens to each", () => {
    expect(
      companyArchiveConfirm(t, plural, "en", company, viewWith(1, 2)),
    ).toBe(
      `Archive ${company.display_name}? ` +
        "Its 1 contact stays, but no longer shows this company as employer. " +
        "Its 2 open deals stay open in the pipeline. " +
        "You can bring it back from its history.",
    );
  });

  it("leaves out what the company does not have", () => {
    expect(
      companyArchiveConfirm(t, plural, "en", company, viewWith(0, 0)),
    ).toBe(
      `Archive ${company.display_name}? You can bring it back from its history.`,
    );
  });

  it("names no number it could not count", () => {
    expect(companyArchiveConfirm(t, plural, "en", company, undefined)).toBe(
      `Archive ${company.display_name}? ` +
        "Its contacts stay, but no longer show this company as employer. " +
        "Its open deals stay open. You can bring it back from its history.",
    );
  });
});
