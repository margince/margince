// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../../i18n/en";
import {
  legalOn,
  queueItem,
  REVIEW_ID,
  review,
  VIEWER_ID,
} from "../../testing/legalreviews.fixtures";
import {
  jsonResponse,
  permissionDenied,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { LegalReviewSettingsCard } from "../settings/legalreviewsettings";
import { LegalReviewsRoute } from "./legalreviews";

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
    // The agreements, latest contact and open tasks come with the pack, and the
    // current offer links to its PDF.
    expect(screen.getByText("Master services agreement")).toBeInTheDocument();
    expect(screen.getByText("60 days")).toBeInTheDocument();
    expect(
      screen.getByText(en["legalReview.contractAutoRenews"]),
    ).toBeInTheDocument();
    expect(screen.getByText("Liability cap call")).toBeInTheDocument();
    // An entry that knows only its day is drawn as a date, never as noon UTC.
    expect(
      screen.getByText(
        (text) => text.startsWith("Note, ") && !text.includes(":"),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Send the data processing terms"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: en["offer.viewPdf"] }),
    ).toHaveAttribute(
      "href",
      "/v1/offers/019f7e65-0000-7000-8000-0000000000f1/pdf",
    );
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

  it("says a pack made before the new sections predates them", async () => {
    const older = review();
    delete older.packet.contracts;
    delete older.packet.touches;
    delete older.packet.tasks;
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        [`GET /legal-reviews/${REVIEW_ID}`]: () => jsonResponse(older),
      },
      REVIEWER,
    );
    mount(REVIEW_ID);

    expect(
      await screen.findByText(en["legalReview.packOlder"]),
    ).toBeInTheDocument();
    expect(screen.queryByText("Master services agreement")).toBeNull();
  });

  it("states auto-renewal on a contract that has no renewal date", async () => {
    const undated = review();
    for (const contract of undated.packet.contracts ?? []) {
      delete contract.renewal_on;
    }
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        [`GET /legal-reviews/${REVIEW_ID}`]: () => jsonResponse(undated),
      },
      REVIEWER,
    );
    mount(REVIEW_ID);

    expect(
      await screen.findByText(en["legalReview.contractAutoRenews"]),
    ).toBeInTheDocument();
    expect(screen.getByText(en["legalReview.contractRenewal"])).toBeVisible();
  });

  it("shows each offer line as the buyer reads it, and the note as written", async () => {
    const shown = review({ note: "Check the cap.\nAnd the venue." });
    const offer = shown.packet.offer;
    if (!offer) throw new Error("the fixture carries an offer");
    offer.lines[0] = { ...offer.lines[0], discount_pct: "12.50" };
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        [`GET /legal-reviews/${REVIEW_ID}`]: () => jsonResponse(shown),
      },
      { ...REVIEWER, deal: ["read"] },
    );
    mount(REVIEW_ID);

    expect(
      await screen.findByText(/^2 × Implementation at €37,815\.13/),
    ).toBeInTheDocument();
    expect(screen.getByText("Discount 12.5%")).toBeInTheDocument();
    expect(screen.getAllByText("Tax 19%")).toHaveLength(2);
    expect(screen.getAllByText(en["legalReview.offerTotal"])).not.toHaveLength(
      0,
    );
    // The deal links only for a reader who may open deals, once that is known.
    await vi.waitFor(() =>
      expect(screen.getByRole("radio", { name: /Approve/ })).toBeEnabled(),
    );
    expect(
      screen.getByRole("link", { name: "Warehouse rollout" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Note: Check the cap.\nAnd the venue.", {
        normalizer: (text) => text,
      }),
    ).toHaveClass("legalreview-note");
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
