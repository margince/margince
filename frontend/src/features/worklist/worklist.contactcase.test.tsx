// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ANA, BEN, contactCase } from "../../testing/contactcase.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { answerBelow } from "./worklist.suggestion";

type WorklistItem = components["schemas"]["WorklistItem"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function row(over: Partial<WorklistItem>): WorklistItem {
  return {
    occurred_precision: "instant",
    id: ANA,
    source: "contact_case",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    because: [],
    actions: ["merge"],
    ...over,
  };
}

const CASE_ROW = row({
  contact_case: {
    anchor_id: ANA,
    revision: "rev-1",
    members: [
      { id: ANA, label: "Mira Kovac", detail: "mira@northwind.de" },
      { id: BEN, label: "Mira Kovač", detail: "m.kovac@northwind.de" },
    ],
    owners: ["Ana Owner", "Ben Seller"],
    can_merge: true,
    merge_mode: "delegated",
  },
});

function draw(item: WorklistItem) {
  const placed = answerBelow(item);
  return render(<StoryProviders>{placed?.below}</StoryProviders>);
}

describe("a contact case on the Worklist", () => {
  it("names the members and owners, and opens the shared review", async () => {
    stubWithSession(
      {
        [`GET /dedupe/contact-cases/${ANA}`]: () => jsonResponse(contactCase()),
      },
      {},
    );
    const user = userEvent.setup();
    draw(CASE_ROW);
    expect(screen.getByText("m.kovac@northwind.de")).toBeVisible();
    expect(
      screen.getByText(
        "Owned by Ana Owner and Ben Seller. Any of them can merge these.",
      ),
    ).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Review" }));
    expect(
      await screen.findByRole("button", { name: "Preview merge" }),
    ).toBeVisible();
  });

  it("mounts for a contact case only, and leaves a pair to the pair card", () => {
    expect(answerBelow(row({ contact_case: undefined }))).toBeUndefined();
    const pair = row({
      source: "dedupe_candidate",
      actions: ["merge", "dismiss"],
      pair: {
        left: { id: ANA, label: "Acme GmbH" },
        right: { id: BEN, label: "ACME Gmbh" },
        evidence: [],
      },
    });
    draw(pair);
    expect(screen.queryByTestId("contact-case-row")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Keep Acme GmbH" }),
    ).toBeVisible();
  });
});
