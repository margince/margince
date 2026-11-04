/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import {
  type Contact360,
  mount,
  recordHeader,
  view,
} from "../../testing/contactpage.testkit";

afterEach(() => {
  cleanup();
});

// The quiet rung with no open task: the Next bar says no follow-up task is
// recorded, and offers one only to a reader whose grant to log is known.
const quiet: Contact360 = {
  ...view,
  moment: {
    claim_key: "moment:nothing_needed",
    evidence_fingerprint: "quiet",
    rule: "nothing_needed",
    headline: "Nothing needs you today",
    why_now: "No meeting coming up, nothing owed, nobody waiting on a reply.",
    confidence: "observed_fact",
    evidence: [],
    recommended_action: {
      kind: "log_activity",
      label: "Log an interaction",
      state: "available",
      destination: { surface: "activity_log" },
    },
  },
};

const addFollowUp = () =>
  screen.queryByRole("button", { name: en["standing.next.addTask"] });

describe("Add follow-up on a quiet contact", () => {
  it("is offered to a reader who may log activity", async () => {
    mount("overview", quiet);
    expect(
      await screen.findByRole("button", { name: en["standing.next.addTask"] }),
    ).toBeTruthy();
  });

  it("is not offered while the reader's grants are still in flight", async () => {
    mount("overview", quiet, [], {
      "GET /me": () => new Promise<Response>(() => {}),
    });
    await recordHeader();
    expect(await screen.findByText(en["standing.next.noTask"])).toBeTruthy();
    expect(addFollowUp()).toBeNull();
  });

  it("is not offered when the reader's grants failed to load", async () => {
    mount("overview", quiet, [], {
      "GET /me": () => new Response(null, { status: 500 }),
    });
    await recordHeader();
    expect(await screen.findByText(en["standing.next.noTask"])).toBeTruthy();
    expect(addFollowUp()).toBeNull();
  });

  it("is not offered to a reader without the grant to log activity", async () => {
    mount("overview", quiet, [], {}, { contact: ["read", "update"] });
    await recordHeader();
    expect(await screen.findByText(en["standing.next.noTask"])).toBeTruthy();
    expect(addFollowUp()).toBeNull();
  });
});
