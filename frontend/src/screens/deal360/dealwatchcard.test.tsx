/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { installFetchStub, meRoute, StoryProviders } from "../story-utils";
import { DealWatchCard, DealWatchList } from "./dealwatchcard";

type DealCommitments = components["schemas"]["DealCommitments"];

const MESSAGE = "01a02e25-a5ac-7099-8099-581cbf001a01";

const ONE: DealCommitments = {
  complete: true,
  has_more: false,
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
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"], contact: ["read", "update"] }),
  });
  render(
    <StoryProviders>
      <DealWatchList commitments={commitments} onOpenEmail={onOpenEmail} />
    </StoryProviders>,
  );
  return onOpenEmail;
}

describe("the deal's watch card", () => {
  it("names who committed, to what, in which words", async () => {
    const open = renderList(ONE);
    expect(
      screen.getByText("Ines Huber: Send the purchase order"),
    ).toBeTruthy();
    expect(
      screen.getByText("“We will send the purchase order by Friday.”"),
    ).toBeTruthy();
    screen.getByRole("button", { name: "Open message" }).click();
    expect(open).toHaveBeenCalledWith(MESSAGE);
    expect(await screen.findByRole("button", { name: "Dismiss" })).toBeTruthy();
  });

  it("offers no message to open for a commitment made in a meeting", () => {
    renderList({
      complete: true,
      data: [{ ...ONE.data[0], source_kind: "meeting" }],
    });
    expect(screen.queryByRole("button", { name: "Open message" })).toBeNull();
  });

  it("says when more are open than it shows", () => {
    renderList({ ...ONE, has_more: true });
    expect(
      screen.getByText(
        "Showing the 25 most urgent commitments; more are open.",
      ),
    ).toBeTruthy();
  });

  it("draws nothing when the customer owes nothing", () => {
    renderList({ complete: true, has_more: false, data: [] });
    expect(screen.queryByText("Customer commitments")).toBeNull();
  });

  it("says when the list is only part of the account", () => {
    renderList({ complete: false, has_more: false, data: [] });
    expect(
      screen.getByText("Some commitments at this company are hidden from you."),
    ).toBeTruthy();
  });
});

describe("the deal's watch card, read from the server", () => {
  it("says a refused read rather than drawing nothing", async () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /deals/d-1/commitments": () =>
        new Response(
          JSON.stringify({
            title: "Forbidden",
            status: 403,
            detail: "no contact grant",
          }),
          {
            status: 403,
            headers: { "content-type": "application/problem+json" },
          },
        ),
    });
    render(
      <StoryProviders>
        <DealWatchCard dealId="d-1" />
      </StoryProviders>,
    );
    expect(await screen.findByRole("alert")).toBeTruthy();
  });
});
