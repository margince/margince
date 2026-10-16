// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../../i18n/en";
import {
  jsonResponse,
  permissionDenied,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { LegalReviewsRoute } from "./legalreviews";
import {
  legalOn,
  queueItem,
  REVIEW_ID,
  review,
  VIEWER_ID,
} from "./legalreviews.fixtures";
import { LegalReviewSettingsCard } from "./legalreviewsettings";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const REVIEWER = { legal_review: ["read", "update"] } as const;

function mount(id?: string) {
  return render(
    <StoryProviders>
      <LegalReviewsRoute id={id} />
    </StoryProviders>,
  );
}

describe("the reviewer's queue", () => {
  it("draws the rows in the order the server sorted them", async () => {
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        "GET /legal-reviews": () =>
          jsonResponse({
            data: [queueItem("Big", 900_000), queueItem("No value", null)],
            page: { has_more: false },
          }),
      },
      REVIEWER,
    );
    mount();
    const big = await screen.findByRole("button", { name: "Big" });
    const none = screen.getByRole("button", { name: "No value" });
    expect(
      big.compareDocumentPosition(none) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});

describe("one review", () => {
  it("records changes needed with the comments", async () => {
    const posted: unknown[] = [];
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        [`GET /legal-reviews/${REVIEW_ID}`]: () => jsonResponse(review()),
        [`POST /legal-reviews/${REVIEW_ID}/decide`]: (body) => {
          posted.push(body);
          return jsonResponse(review({ status: "changes_needed" }).review);
        },
      },
      REVIEWER,
    );
    const user = userEvent.setup();
    mount(REVIEW_ID);

    expect(await screen.findByText("msa-draft.pdf")).toBeInTheDocument();
    // Every term the approval binds to is on the page.
    expect(screen.getByText("Thank you for the meeting.")).toBeInTheDocument();
    expect(screen.getByText("Monthly for 36 periods")).toBeInTheDocument();
    expect(
      screen.getByText(en["legalReview.offerLineStaged"]),
    ).toBeInTheDocument();
    expect(screen.getByText(/OF-2026-015/)).toBeInTheDocument();
    // A file this reviewer cannot open stays listed, with no name of its own.
    expect(
      screen.getByText(en["legalReview.docUnavailable"]),
    ).toBeInTheDocument();
    await user.click(
      await screen.findByRole("radio", {
        name: new RegExp(en["legalReview.outcomeChanges"]),
      }),
    );
    const record = screen.getByRole("button", {
      name: en["legalReview.decide"],
    });
    expect(record).toBeDisabled();
    await user.type(
      screen.getByLabelText(en["legalReview.commentsLabel"]),
      "Cap the liability.",
    );
    await user.click(record);
    await vi.waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({
      outcome: "changes_needed",
      comments: "Cap the liability.",
    });
  });

  it("does not let the requester answer their own review", async () => {
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        [`GET /legal-reviews/${REVIEW_ID}`]: () =>
          jsonResponse(review({ requested_by: VIEWER_ID })),
      },
      REVIEWER,
    );
    mount(REVIEW_ID);
    expect(
      await screen.findByText(en["legalReview.ownRequest"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["legalReview.decide"] }),
    ).toBeDisabled();
  });
});

describe("the installation's switch", () => {
  // Ops holds installation_settings too, and the server still refuses it.
  it.each([
    ["ops", false],
    ["admin", true],
  ])(
    "is live for the %s role only if the server takes it",
    async (role, live) => {
      stubWithSession(
        { "GET /legal-reviews/settings": legalOn },
        { installation_settings: ["read", "update"] },
        { roles: [role] },
      );
      render(
        <StoryProviders>
          <LegalReviewSettingsCard />
        </StoryProviders>,
      );
      const toggle = await screen.findByRole("switch");
      if (live) expect(toggle).toBeEnabled();
      else expect(toggle).toBeDisabled();
    },
  );

  it("names the card's notice on the switch when the server refuses the flip", async () => {
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        "PUT /legal-reviews/settings": permissionDenied,
      },
      { installation_settings: ["read", "update"] },
      { roles: ["admin"] },
    );
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <LegalReviewSettingsCard />
      </StoryProviders>,
    );
    const toggle = await screen.findByRole("switch");
    expect(toggle).not.toHaveAttribute("aria-describedby");
    await user.click(toggle);
    await screen.findByText(en["legalReview.notSaved"]);
    expect(toggle).toHaveAccessibleDescription(
      expect.stringContaining(en["legalReview.notSaved"]),
    );
  });
});
