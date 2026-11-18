// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */

import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { useConnectors } from "../../lib/connectors";
import { renderWithQueryClient } from "../../testing/queryrender";
import { stubWithSession } from "../../testing/story-utils";
import { FromMailboxRow } from "./compose.from";

type CaptureConnection = components["schemas"]["CaptureConnection"];

const work: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  can_backfill: true,
  status: "connected",
  scopes: ["https://www.googleapis.com/auth/gmail.send"],
  account_label: "rep@acme.test",
};
const second: CaptureConnection = {
  ...work,
  id: "018f3a1b-0000-7000-8000-0000000000c2",
  provider: "graph",
  scopes: ["Mail.Send"],
  account_label: "rep@second.test",
};

function stubRoster(data: CaptureConnection[]) {
  stubWithSession({ "GET /connectors": () => Response.json({ data }) }, {});
}

/** Says when the roster the row reads from has answered. */
function RosterRead() {
  return useConnectors().data ? <p>roster read</p> : null;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the From choice", () => {
  it("offers each sending mailbox when the sender holds more than one", async () => {
    stubRoster([work, second]);
    const onChange = vi.fn();
    renderWithQueryClient(<FromMailboxRow value="" onChange={onChange} />);
    await userEvent.click(await screen.findByTestId("compose-from-mailbox"));
    await userEvent.click(
      await screen.findByRole("option", { name: "rep@second.test" }),
    );
    expect(onChange).toHaveBeenCalledWith(second.id);
  });

  it("draws nothing for a sender with one mailbox, or a calendar beside it", async () => {
    stubRoster([
      work,
      { ...second, provider: "gcal" },
      { ...second, id: "018f3a1b-0000-7000-8000-0000000000c3", scopes: [] },
    ]);
    renderWithQueryClient(
      <>
        <RosterRead />
        <FromMailboxRow value="" onChange={vi.fn()} />
      </>,
    );
    await screen.findByText("roster read");
    expect(screen.queryByTestId("compose-from-mailbox")).toBeNull();
  });
});
