/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { labelsIn } from "../testing/shippedlabels.testkit";
import { useAcquisitionSources } from "./acquisitionsources.queries";
import { useLeadOrigins } from "./leadorigins";
import { shippedLabel } from "./shippedlabels";

afterEach(() => {
  vi.unstubAllGlobals();
});

const inVietnamese = (key: MessageKey) => translate("vi", key);

function row(key: string, label: string, labelKey: string | null) {
  return {
    id: `row-${key}`,
    key,
    label,
    label_key: labelKey,
    intent: "neutral",
    sort_order: 10,
    active: true,
    system: labelKey !== null,
    lead_count: 0,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

describe("a built-in label", () => {
  it("reads from the catalog while the server names its key", () => {
    expect(
      shippedLabel(
        { label: "Inbound", label_key: "acqSources.builtin.inbound" },
        inVietnamese,
      ),
    ).toBe("Khách tự liên hệ");
  });

  it("reads as typed once an administrator relabels it", () => {
    expect(
      shippedLabel({ label: "Website chat", label_key: null }, inVietnamese),
    ).toBe("Website chat");
  });

  it("reads as stored when the key is one this build does not ship", () => {
    expect(
      shippedLabel(
        { label: "Trade fair", label_key: "acqSources.builtin.tradeFair" },
        inVietnamese,
      ),
    ).toBe("Trade fair");
  });
});

describe("the administered lists, read in Vietnamese", () => {
  it("names the deal sources in the reader's language, an added one as typed", async () => {
    const labels = await labelsIn(
      "vi",
      "/acquisition-sources",
      [
        row("inbound", "Inbound", "acqSources.builtin.inbound"),
        row(
          "existing_customer",
          "Existing customer",
          "acqSources.builtin.existing_customer",
        ),
        row("webinar", "Webinar", null),
      ],
      useAcquisitionSources,
    );
    expect(labels).toEqual([
      "Khách tự liên hệ",
      "Khách hàng hiện tại",
      "Webinar",
    ]);
  });

  it("names the lead origins in the reader's language, a renamed one as typed", async () => {
    const labels = await labelsIn(
      "vi",
      "/lead-origins",
      [
        row("referral", "Referral", "leadOrigins.builtin.referral"),
        row("event", "Trade fairs", null),
      ],
      useLeadOrigins,
    );
    expect(labels).toEqual(["Giới thiệu", "Trade fairs"]);
  });
});
