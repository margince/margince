// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../../i18n/en";
import { useLegalReviewAvailable } from "../../../lib/legalreviews.queries";
import { useMe } from "../../../lib/session";
import {
  changesNeeded,
  DEAL_ID,
  EARLIER_ID,
  legalDeal,
  legalOff,
  legalOn,
  summary,
} from "../../../testing/legalreviews.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../../testing/story-utils";
import { DealLegalTab, RequestLegalReviewAction } from "./deallegaltab";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DEAL_WRITER = { deal: ["read", "update"] } as const;

/** Says so once both answers the verb reads have arrived. */
function Settled() {
  const available = useLegalReviewAvailable();
  const me = useMe();
  return available !== undefined && me.data ? <p>settled</p> : null;
}

function mount(node: React.ReactNode) {
  return render(<StoryProviders>{node}</StoryProviders>);
}

describe("a deal's legal review tab", () => {
  it("shows legal's comments on a review that asked for changes, and asks again linked to it", async () => {
    const posted: unknown[] = [];
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        [`GET /deals/${DEAL_ID}/legal-reviews`]: () =>
          jsonResponse({ data: [changesNeeded] }),
        [`POST /deals/${DEAL_ID}/legal-reviews`]: (body) => {
          posted.push(body);
          return jsonResponse(summary(), 201);
        },
      },
      DEAL_WRITER,
    );
    const user = userEvent.setup();
    mount(<DealLegalTab deal={legalDeal} />);

    expect(
      await screen.findByText("Cap the liability at the contract value."),
    ).toBeInTheDocument();
    await user.click(
      await screen.findByRole("button", {
        name: en["legalReview.requestAgain"],
      }),
    );
    await user.type(
      await screen.findByLabelText(en["legalReview.noteLabel"]),
      "Cap changed",
    );
    await user.click(
      screen.getByRole("button", { name: en["legalReview.requestConfirm"] }),
    );
    await vi.waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({
      note: "Cap changed",
      previous_review_id: EARLIER_ID,
    });
  });

  it("offers no request while one waits for legal", async () => {
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        [`GET /deals/${DEAL_ID}/legal-reviews`]: () =>
          jsonResponse({ data: [summary()] }),
      },
      DEAL_WRITER,
    );
    mount(<DealLegalTab deal={legalDeal} />);
    expect(
      await screen.findByText(en["legalReview.status.requested"]),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["legalReview.request"] }),
    ).toBeNull();
    expect(
      await screen.findByRole("button", { name: en["legalReview.withdraw"] }),
    ).toBeInTheDocument();
  });

  it("says there is no review yet, and still offers the first request", async () => {
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        [`GET /deals/${DEAL_ID}/legal-reviews`]: () =>
          jsonResponse({ data: [] }),
      },
      DEAL_WRITER,
    );
    mount(<DealLegalTab deal={legalDeal} />);
    expect(
      await screen.findByText(en["legalReview.empty"]),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("button", { name: en["legalReview.request"] }),
    ).toBeInTheDocument();
  });

  it("shows the requester's note in the comments of a review legal has not answered", async () => {
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        [`GET /deals/${DEAL_ID}/legal-reviews`]: () =>
          jsonResponse({ data: [summary({ note: "Check the cap" })] }),
      },
      DEAL_WRITER,
    );
    mount(<DealLegalTab deal={legalDeal} />);
    const table = await screen.findByRole("table", { name: "Reviews" });
    expect(within(table).getByText("Note: Check the cap")).toBeInTheDocument();
    expect(within(table).queryByText("Former colleague")).toBeNull();
  });
});

describe("the request verb", () => {
  it("is not offered while legal review is switched off", async () => {
    stubWithSession({ "GET /legal-reviews/settings": legalOff }, DEAL_WRITER);
    mount(
      <>
        <Settled />
        <RequestLegalReviewAction deal={legalDeal} />
      </>,
    );
    await screen.findByText("settled");
    expect(
      screen.queryByRole("button", { name: en["legalReview.request"] }),
    ).toBeNull();
  });

  it("is not offered on a deal the reader may not change", async () => {
    stubWithSession(
      { "GET /legal-reviews/settings": legalOn },
      { deal: ["read"] },
    );
    mount(
      <>
        <Settled />
        <RequestLegalReviewAction deal={legalDeal} />
      </>,
    );
    await screen.findByText("settled");
    expect(
      screen.queryByRole("button", { name: en["legalReview.request"] }),
    ).toBeNull();
  });
});
