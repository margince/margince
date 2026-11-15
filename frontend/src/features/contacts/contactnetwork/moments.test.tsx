/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { en } from "../../../i18n/en";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import type { IntroRequest } from "../introrequests";
import { MomentsPanel } from "./moments";

type Moments = Pick<
  components["schemas"]["Contact360"],
  "relationship_changes" | "sections_omitted"
>;

// Progress ends on one dated line, so an earlier ask sits where it happened
// among the changes rather than in a list of its own above them.

function ask(over: Partial<IntroRequest> = {}): IntroRequest {
  return {
    id: "ir-1",
    contact_id: "p-1",
    origin: "asked",
    requester_user_id: "u-requester",
    introducer_user_id: "u-introducer",
    route_type: "direct",
    internal_reason: "she ran the migration we are pitching",
    status: "declined",
    name_drop_allowed: false,
    fallback_policy: "none",
    note_generated_by: "human",
    note_ai_generated: false,
    requested_at: "2026-08-20T09:00:00Z",
    decided_at: "2026-08-30T09:00:00Z",
    version: 1,
    ...over,
  };
}

const moved: Moments = {
  sections_omitted: [],
  relationship_changes: [
    { kind: "replied_after_gap", at: "2026-09-02T09:00:00Z", days: 41 },
    {
      kind: "warmed",
      at: "2026-08-27T09:00:00Z",
      from_bucket: "weak",
      to_bucket: "moderate",
    },
  ],
};

function renderTimeline(view: Moments, asks: IntroRequest[]) {
  render(
    <StoryProviders>
      <MomentsPanel view={view} asks={asks} />
    </StoryProviders>,
  );
}

function rowTexts(): string[] {
  return Array.from(document.querySelectorAll(".pn-moment")).map(
    (row) => row.textContent ?? "",
  );
}

beforeEach(() => {
  installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) });
});

afterEach(() => {
  cleanup();
});

describe("the Progress timeline", () => {
  it("places an earlier ask by date among the changes, newest first", () => {
    renderTimeline(moved, [ask()]);
    const rows = rowTexts();
    expect(rows).toHaveLength(3);
    expect(rows[0]).toContain("41");
    expect(rows[1]).toContain(en["contact.intro.stateDeclined"]);
    expect(rows[1]).toContain("she ran the migration we are pitching");
  });

  it("keeps the latest-change flag on the newest change, even under a newer ask", () => {
    renderTimeline(moved, [ask({ decided_at: "2026-09-10T09:00:00Z" })]);
    const rows = rowTexts();
    expect(rows[0]).toContain(en["contact.intro.stateDeclined"]);
    expect(rows[0]).not.toContain(en["contact.intro.stripWhyNow"]);
    expect(rows[1]).toContain(en["contact.intro.stripWhyNow"]);
    expect(screen.getAllByText(en["contact.intro.stripWhyNow"])).toHaveLength(
      1,
    );
  });

  it("renders a name-drop as a name-drop and never as an introduction", () => {
    renderTimeline(moved, [
      ask({ status: "name_dropped", name_dropped_at: "2026-08-28T09:00:00Z" }),
    ]);
    expect(screen.getByText(en["contact.intro.stateNameDropped"])).toBeTruthy();
    expect(screen.queryByText(en["contact.intro.stateIntroduced"])).toBeNull();
  });

  it("renders a recorded introduction without a reason or any verb", () => {
    renderTimeline(moved, [
      ask({
        origin: "recorded",
        recorded_channel: "in_person",
        status: "introduced",
        internal_reason: null,
        decided_at: null,
        introduced_at: "2026-08-30T00:00:00Z",
      }),
    ]);
    expect(screen.getByText(en["contact.intro.recordedBadge"])).toBeTruthy();
    expect(screen.getByText(/Face to face/)).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("still shows earlier asks when the changes are withheld, and says they are", () => {
    renderTimeline(
      { sections_omitted: ["relationship_changes"], relationship_changes: [] },
      [ask()],
    );
    expect(rowTexts()).toHaveLength(1);
    expect(screen.getByText("Hidden for your role")).toBeTruthy();
    expect(screen.queryByText(en["contact.network.noMoments"])).toBeNull();
  });

  it("says nothing has moved only when there is neither a change nor an ask", () => {
    renderTimeline({ sections_omitted: [], relationship_changes: [] }, []);
    expect(screen.getByText(en["contact.network.noMoments"])).toBeTruthy();
  });
});
