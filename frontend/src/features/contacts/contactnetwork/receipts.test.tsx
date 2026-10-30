// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { ReceiptList } from "./receipts";

type Receipt = components["schemas"]["ContactGraphReceipt"];

const OWN_MAIL: Receipt = {
  activity_id: "018f3a1b-0000-7000-8000-0000000000e1",
  subject: "Re: Q4 rollout, site list",
  occurred_at: "2026-08-28T09:00:00Z",
  occurred_precision: "instant",
  kind: "email",
};

const RELAYED: Receipt = {
  activity_id: "018f3a1b-0000-7000-8000-0000000000e3",
  subject: "Information about your personal data",
  occurred_at: "2026-10-20T09:00:00Z",
  occurred_precision: "instant",
  kind: "email",
  controller_send: { from_address: "privacy@kps.test" },
};

function renderList(
  receipts: readonly Receipt[],
  relayMember?: "self" | "colleague",
) {
  render(
    <LocaleProvider initial="en">
      <ReceiptList receipts={receipts} relayMember={relayMember} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("a network receipt the installation's relay sent", () => {
  it("says Margince sent it for the reader, and from which address", () => {
    renderList([OWN_MAIL, RELAYED], "self");

    const relayed = screen
      .getByText("Information about your personal data")
      .closest("li");
    expect(relayed).toHaveTextContent(en["trust.relayForYou"]);
    expect(relayed).toHaveTextContent("From privacy@kps.test");
    expect(
      screen.getByText("Re: Q4 rollout, site list").closest("li"),
    ).not.toHaveTextContent(/Sent by Margince|From /);
  });

  it("says it was sent for a colleague on a colleague's relationship", () => {
    renderList([RELAYED], "colleague");

    expect(screen.getByText(en["trust.relayForColleague"])).toBeVisible();
    expect(screen.queryByText(en["trust.relayForYou"])).toBeNull();
  });

  it("claims no member when the list cannot say whose relationship it is", () => {
    renderList([{ ...RELAYED, controller_send: { from_address: null } }]);

    expect(screen.getByText(en["trust.relaySent"])).toBeVisible();
    expect(screen.queryByText(/^From /)).toBeNull();
  });
});
