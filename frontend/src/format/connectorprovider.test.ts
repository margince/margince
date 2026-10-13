// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { accountNameOf, accountsOf } from "./connectorprovider";

type CaptureConnection = components["schemas"]["CaptureConnection"];

function connection(
  provider: CaptureConnection["provider"],
): CaptureConnection {
  return {
    id: `id-${provider}`,
    provider,
    can_backfill: false,
    status: "connected",
    scopes: [],
  };
}

/** Each account as `key: mail+calendar`, so one assertion reads the grouping. */
function shape(rows: readonly CaptureConnection[]): string[] {
  return accountsOf(rows).map(
    (account) =>
      `${account.key}: ${account.mail?.provider ?? "-"}+${account.calendar?.provider ?? "-"}`,
  );
}

describe("accountsOf", () => {
  it("folds Gmail and Google Calendar into one Google account", () => {
    expect(shape([connection("gcal"), connection("gmail")])).toEqual([
      "google: gmail+gcal",
    ]);
  });

  it("folds Outlook mail and calendar into one Microsoft account", () => {
    expect(shape([connection("graph"), connection("graphcal")])).toEqual([
      "microsoft: graph+graphcal",
    ]);
  });

  it("keeps IMAP and the test mailbox as accounts of their own, with no calendar", () => {
    expect(shape([connection("imap"), connection("test_mailbox")])).toEqual([
      "imap: imap+-",
      "test_mailbox: test_mailbox+-",
    ]);
  });

  it("keeps a calendar with no mailbox beside it as an account", () => {
    expect(shape([connection("graphcal")])).toEqual(["microsoft: -+graphcal"]);
  });

  // The server lists rows by provider name, so gcal arrives before gmail and
  // imap before test_mailbox: the account order must not follow that.
  it("puts Google first and Microsoft second, then the rest in roster order", () => {
    expect(
      shape([
        connection("test_mailbox"),
        connection("graph"),
        connection("imap"),
        connection("gcal"),
      ]),
    ).toEqual([
      "google: -+gcal",
      "microsoft: graph+-",
      "test_mailbox: test_mailbox+-",
      "imap: imap+-",
    ]);
  });

  it("returns no accounts for an empty roster", () => {
    expect(accountsOf([])).toEqual([]);
  });
});

describe("accountNameOf", () => {
  // A caption that names where a service came from names its account, so the
  // mailbox and the calendar of one vendor read the same.
  it("names both services of a vendor by the vendor, and the rest by kind", () => {
    const providers: CaptureConnection["provider"][] = [
      "gmail",
      "gcal",
      "graph",
      "graphcal",
      "imap",
      "test_mailbox",
    ];
    expect(providers.map(accountNameOf)).toEqual([
      "connectors.vendorGoogle",
      "connectors.vendorGoogle",
      "connectors.vendorMicrosoft",
      "connectors.vendorMicrosoft",
      "connectors.provImap",
      "connectors.provTestMailbox",
    ]);
  });
});
