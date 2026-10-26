/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import {
  block,
  CONNECTED,
  jsonResponse,
  openPanel,
  panel,
  ROUTE,
  render,
  settlesOnLine,
  stubAgentRailApi,
} from "./agentrail.testkit";

// One Google account is two connections: a Gmail grant and a Calendar grant,
// both labelled with the account's address. The panel used to reduce each to
// that address and key the row by it, so the two offline sources rendered as
// one ambiguous line twice and React warned about a duplicate key.
const ACCOUNT = "rep@seller.example";
const GMAIL = {
  ...CONNECTED,
  id: "018f3a1b-0000-7000-8000-00000000a001",
  provider: "gmail" as const,
  account_label: ACCOUNT,
};
const GCAL = {
  ...CONNECTED,
  id: "018f3a1b-0000-7000-8000-00000000a002",
  provider: "gcal" as const,
  account_label: ACCOUNT,
  scopes: ["https://www.googleapis.com/auth/calendar.readonly"],
};

const sourceRows = () =>
  Array.from(panel().querySelectorAll(".arconn")).map((el) => el.textContent);

const duplicateKeyWarnings = (spy: ReturnType<typeof vi.spyOn>) =>
  spy.mock.calls.filter((call: unknown[]) =>
    call.some((arg) => String(arg).includes("same key")),
  );

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the agent panel's offline sources", () => {
  it("names a Gmail and a Calendar grant of one account as two sources, without a key collision", async () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    const user = userEvent.setup();
    stubAgentRailApi({
      connectors: () =>
        jsonResponse({
          data: [
            { ...GMAIL, status: "reauth_required" },
            { ...GCAL, status: "error" },
          ],
        }),
    });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    await openPanel(user, container);
    await waitFor(() =>
      expect(sourceRows()).toEqual([
        `${ACCOUNT} (${en["connectors.provGmail"]}) ${en["agent.fact.offline"]}`,
        `${ACCOUNT} (${en["connectors.provGcal"]}) ${en["agent.fact.offline"]}`,
      ]),
    );
    expect(duplicateKeyWarnings(errors)).toEqual([]);
  });

  it("lists only the connection that is down when its sibling is connected", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      connectors: () =>
        jsonResponse({ data: [GMAIL, { ...GCAL, status: "reauth_required" }] }),
    });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(sourceRows()).toEqual([
        `${ACCOUNT} (${en["connectors.provGcal"]}) ${en["agent.fact.offline"]}`,
      ]),
    );
  });

  it("falls back to the provider's name for a connection with no address", async () => {
    const user = userEvent.setup();
    const { account_label: _gmail, ...gmail } = GMAIL;
    const { account_label: _gcal, ...gcal } = GCAL;
    stubAgentRailApi({
      connectors: () =>
        jsonResponse({
          data: [
            { ...gmail, status: "error" },
            { ...gcal, status: "error" },
          ],
        }),
    });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(sourceRows()).toEqual([
        `${en["connectors.provGmail"]} ${en["agent.fact.offline"]}`,
        `${en["connectors.provGcal"]} ${en["agent.fact.offline"]}`,
      ]),
    );
  });

  it("says nothing needs attention when every connection is connected", async () => {
    stubAgentRailApi({
      connectors: () => jsonResponse({ data: [GMAIL, GCAL] }),
    });
    const { container } = render(ROUTE);
    await settlesOnLine(container, en["agent.line.allClear"]);
  });
});
