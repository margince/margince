// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { en } from "../../i18n/en";
import { RecordZoneProvider } from "../../lib/recordzone";
import { StoryProviders } from "../../testing/story-utils";
import { ContactClaimsReview } from "./contactclaimsreview";

type Claim = components["schemas"]["ConversationClaim"];

afterEach(cleanup);

const claim = (over: Partial<Claim>): Claim => ({
  id: "claim-1",
  kind: "commitment_ours",
  status: "open",
  needs_review: true,
  body: "Arrange the rollout meeting",
  source_activity_id: "a-1",
  source_quote: "I will arrange the rollout meeting.",
  source_label: "Rollout planning",
  occurred_at: "2026-07-05T05:00:00Z",
  occurred_precision: "instant",
  ...over,
});

async function caption(row: Claim): Promise<string> {
  render(
    <StoryProviders>
      <RecordZoneProvider zone={UTC_ZONE}>
        <ContactClaimsReview claims={[row]} dismiss={() => null} />
      </RecordZoneProvider>
    </StoryProviders>,
  );
  await userEvent.click(screen.getByText(en["contact.loops.review"]));
  return screen.getByText(/Rollout planning/).textContent ?? "";
}

describe("ContactClaimsReview", () => {
  // A claim read from a date-only note names its day: the noon it is stored
  // at is not a time anybody said.
  it("dates a claim from a date-only source without a clock", async () => {
    const text = await caption(claim({ occurred_precision: "day" }));
    expect(text).toBe("Rollout planning · 05/07/2026");
  });

  it("keeps the time of a claim from a timed source", async () => {
    const text = await caption(claim({}));
    expect(text).toBe("Rollout planning · 05/07/2026, 05:00");
  });
});
