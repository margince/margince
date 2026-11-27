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

type SharedChoice = components["schemas"]["ReplyableSharedMailbox"];

const sales: SharedChoice = {
  id: "018f3a1b-0000-7000-8000-0000000000d1",
  provider: "graph",
  address: "sales@acme.test",
};

function stubRoster(data: CaptureConnection[], shared: SharedChoice[] = []) {
  stubWithSession(
    {
      "GET /connectors": () => Response.json({ data }),
      "GET /shared-mailboxes/replyable": () => Response.json({ items: shared }),
    },
    {},
  );
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
    const user = userEvent.setup();
    stubRoster([work, second]);
    const onChange = vi.fn();
    renderWithQueryClient(<FromMailboxRow value="" onChange={onChange} />);
    await user.click(await screen.findByTestId("compose-from-mailbox"));
    await user.click(
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

  it("offers a shared mailbox by its address, marked shared, and sends its id once chosen", async () => {
    const user = userEvent.setup();
    stubRoster([work], [sales]);
    const onChange = vi.fn();
    renderWithQueryClient(<FromMailboxRow value="" onChange={onChange} />);
    await user.click(await screen.findByTestId("compose-from-mailbox"));
    expect(onChange).not.toHaveBeenCalled();
    await user.click(
      await screen.findByRole("option", {
        name: "sales@acme.test · Shared mailbox",
      }),
    );
    expect(onChange).toHaveBeenCalledWith(sales.id);
  });

  it("keeps the automatic choice first, so a shared mailbox is never preselected", async () => {
    const user = userEvent.setup();
    stubRoster([work], [sales]);
    renderWithQueryClient(<FromMailboxRow value="" onChange={vi.fn()} />);
    await user.click(await screen.findByTestId("compose-from-mailbox"));
    const options = await screen.findAllByRole("option");
    expect(options[0]).toHaveTextContent(
      "Mailbox the conversation arrived in, or your default",
    );
    expect(options).toHaveLength(3);
  });
});
