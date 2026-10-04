/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { installFetchStub, meRoute, StoryProviders } from "../story-utils";
import { DealWatchList } from "./dealwatchcard";

type DealCommitments = components["schemas"]["DealCommitments"];

const MESSAGE = "01a02e25-a5ac-7099-8099-581cbf001a01";

const ONE: DealCommitments = {
  complete: true,
  data: [
    {
      id: "01a02e25-a5ac-7099-8099-581cbf001a02",
      contact_id: "01a02e25-a5ac-7099-8099-581cbf001a03",
      contact_name: "Ines Huber",
      body: "Send the purchase order",
      source_quote: "We will send the purchase order by Friday.",
      source_activity_id: MESSAGE,
      source_kind: "email",
      due_at: null,
      occurred_at: "2026-09-01T08:00:00Z",
    },
  ],
};

afterEach(cleanup);

function renderList(commitments: DealCommitments, onOpenEmail = vi.fn()) {
  installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
  render(
    <StoryProviders>
      <DealWatchList commitments={commitments} onOpenEmail={onOpenEmail} />
    </StoryProviders>,
  );
  return onOpenEmail;
}

describe("the deal's watch card", () => {
  it("names who committed, to what, in which words", () => {
    const open = renderList(ONE);
    expect(
      screen.getByText("Ines Huber: Send the purchase order"),
    ).toBeTruthy();
    expect(
      screen.getByText("“We will send the purchase order by Friday.”"),
    ).toBeTruthy();
    screen.getByRole("button", { name: "Open message" }).click();
    expect(open).toHaveBeenCalledWith(MESSAGE);
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeTruthy();
  });

  it("offers no message to open for a commitment made in a meeting", () => {
    renderList({
      complete: true,
      data: [{ ...ONE.data[0], source_kind: "meeting" }],
    });
    expect(screen.queryByRole("button", { name: "Open message" })).toBeNull();
  });

  it("draws nothing when the customer owes nothing", () => {
    renderList({ complete: true, data: [] });
    expect(screen.queryByText("Customer commitments")).toBeNull();
  });

  it("says when the list is only part of the account", () => {
    renderList({ complete: false, data: [] });
    expect(
      screen.getByText("Some commitments on this account are hidden from you."),
    ).toBeTruthy();
  });
});
