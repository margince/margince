// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  addedViaFor,
  type LeadOrigin,
  originFilterOptions,
  originLabelFor,
  originPickOptions,
  writtenByLabel,
} from "./leadorigins";

const t = ((key: string, params?: Record<string, string>) =>
  params?.uploadName ? `${key}:${params.uploadName}` : key) as Parameters<
  typeof addedViaFor
>[1];

function origin(key: string, label: string, active = true): LeadOrigin {
  return {
    id: `origin-${key}`,
    key,
    label,
    intent: "neutral",
    sort_order: 10,
    active,
    system: false,
    lead_count: 0,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

const LIST = [
  origin("referral", "Referral"),
  origin("trade_show", "Trade show", false),
];

describe("the origin reader", () => {
  it("names an origin by the list's label, else the server's, else the key", () => {
    expect(
      originLabelFor({ origin: "referral", origin_label: "Referral" }, [
        origin("referral", "Giới thiệu"),
      ]),
    ).toBe("Giới thiệu");
    expect(
      originLabelFor({ origin: "unlisted_key", origin_label: "Webinar" }, LIST),
    ).toBe("Webinar");
    expect(originLabelFor({ origin: "referral" }, LIST)).toBe("Referral");
    expect(originLabelFor({ origin: "unlisted_key" }, LIST)).toBe(
      "unlisted_key",
    );
    expect(originLabelFor({ origin: null }, LIST)).toBeUndefined();
  });

  it("offers the active origins, and a retired one the lead already holds", () => {
    expect(originPickOptions(LIST, null)).toEqual([
      { value: "referral", label: "Referral" },
    ]);
    expect(originPickOptions(LIST, "trade_show")[0]).toEqual({
      value: "trade_show",
      label: "Trade show",
    });
  });

  it("keeps a retired origin in the filter, labelled, because leads still carry it", () => {
    expect(originFilterOptions(LIST, "(retired)")).toEqual([
      { value: "referral", text: "Referral" },
      { value: "trade_show", text: "Trade show (retired)" },
    ]);
  });

  it("treats an absent list as an empty one", () => {
    expect(originPickOptions(undefined, null)).toEqual([]);
  });
});

describe("the added-via reader", () => {
  it("says an import wrote the row, with the upload's name", () => {
    expect(
      addedViaFor({ source: "mirror:csv:crm:lead:1", imported_from: "crm" }, t),
    ).toBe("trust.importedFrom:crm");
    expect(addedViaFor({ source: "import", imported_from: "" }, t)).toBe(
      "trust.imported",
    );
  });

  it("names a typed lead and a connector, never the raw token", () => {
    expect(addedViaFor({ source: "manual", imported_from: null }, t)).toBe(
      "lead.source.manual",
    );
    expect(addedViaFor({ source: "siteread", imported_from: null }, t)).toBe(
      "lead.source.siteread",
    );
    expect(
      addedViaFor({ source: "connector:apollo:a-1", imported_from: null }, t),
    ).toBe("Apollo");
  });

  it("reads an import's stamp as an import when only the source value is known", () => {
    expect(writtenByLabel("mirror:csv:crm:lead:1", t)).toBe(
      "trust.importedFrom:crm",
    );
    expect(writtenByLabel(null, t)).toBe("lead.shortfall.noSource");
  });

  it("names a system a record came from as its maker spells it", () => {
    expect(writtenByLabel("hubspot", t)).toBe("HubSpot");
    expect(writtenByLabel("connector:salesforce:s-1", t)).toBe("Salesforce");
  });
});
