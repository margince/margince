/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { DealViabilityView } from "./dealviabilitycard";

type DealViability = components["schemas"]["DealViability"];

afterEach(cleanup);

const EXCLUDED: DealViability = {
  deal_id: "d-1",
  qualification_mode: "enforcing",
  qualification: "excluded",
  disposition: "open_review",
  complete: true,
  assessed_at: "2026-11-02T08:00:00Z",
  close_date_provisional: false,
  close_date_overdue: false,
  stage_probability: 60,
  review: {
    id: "r-1",
    opened_at: "2026-11-01T08:00:00Z",
    episode: 1,
    delivery_state: "delivered",
    task: { id: "t-1", done: true },
  },
};

function draw(viability: DealViability) {
  render(
    <LocaleProvider initial="en">
      <DealViabilityView viability={viability} />
    </LocaleProvider>,
  );
}

describe("DealViabilityView", () => {
  it("keeps an open review after its task is done", () => {
    draw(EXCLUDED);
    expect(
      screen.getByText(en["deals.viability.qualification.excluded"]),
    ).toBeTruthy();
    expect(
      screen.getByText(en["deals.viability.taskDoneReviewOpen"]),
    ).toBeTruthy();
    expect(screen.getByText(en["deals.viability.enforced"])).toBeTruthy();
  });

  it("shows no review row for a supported deal", () => {
    draw({ ...EXCLUDED, qualification: "eligible", review: undefined });
    expect(
      screen.getByText(en["deals.viability.qualification.eligible"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["deals.viability.review"])).toBeNull();
  });

  it("names a closed deal as outside the forecast", () => {
    draw({ ...EXCLUDED, qualification: "closed", review: undefined });
    expect(
      screen.getByText(en["deals.viability.qualification.closed"]),
    ).toBeTruthy();
  });

  it("says when the last check no longer counts", () => {
    draw({
      ...EXCLUDED,
      qualification: "unknown",
      receipt: {
        outcome: "eligible",
        reasons: [],
        valid_until: "2026-11-02T08:00:00Z",
        policy_version: "v1",
        current: false,
      },
    });
    expect(screen.getByText(en["deals.viability.checkOutOfDate"])).toBeTruthy();
  });
});
