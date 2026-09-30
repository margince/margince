/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ContactBriefCard } from "./contactbrief";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

type Contact360 = components["schemas"]["Contact360"];
type ContactBrief = components["schemas"]["ContactBrief"];
type WrittenByWriter = components["schemas"]["WrittenBy"];

// What the relationship brief claims about itself.
//
// The card is a MACHINE's reading in every state it can be in, and that is a
// fact about the feature rather than about the day: the tint and the disclosure
// must not come and go with whichever writer answered, because a card that goes
// plain on the composition tells a reader the composition was somebody's own
// work. Which writer ran is still on the card, in the foot, where a reader
// checks sourcing.

const AT = "2026-08-13T09:00:00Z";
const CONTACT = "3f7c1a90-0000-4000-8000-00000000c001";

const view: Contact360 = {
  as_of: AT,
  contact: {
    id: CONTACT,
    full_name: "Dana Buyer",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: AT,
  },
  sections_omitted: [],
  // Present and empty rather than absent: absent means "you may not see
  // deals", and the band under the prose says the two differently.
  commercial: { role: null, committee: [] },
  claims: [],
};

function brief(by: WrittenByWriter): ContactBrief {
  return {
    contact_id: CONTACT,
    generated_at: AT,
    generated_by: by,
    sentences: [{ text: "They asked about the pilot.", evidence: [] }],
  };
}

beforeEach(() => {
  installFetchStub({ "GET /me": meRoute({}) });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function card(by: WrittenByWriter | undefined) {
  const { container } = render(
    <StoryProviders>
      <ContactBriefCard
        brief={by ? brief(by) : undefined}
        loading={false}
        view={view}
      />
    </StoryProviders>,
  );
  const panel = container.querySelector(".panel");
  expect(panel).not.toBeNull();
  return panel as HTMLElement;
}

describe("the relationship brief discloses the machine that reads it", () => {
  // The claim is the panel's AI tone plus the WrittenBy mark in its head,
  // once. No "AI-assisted" pill on top of those: the tone already says a
  // machine wrote the prose, and the mark names which one.
  it("discloses a model's prose by naming the writer, once", () => {
    const panel = card("model");
    expect(panel.classList).toContain("panel-ai");
    expect(within(panel).queryByText("AI-assisted")).toBeNull();
    expect(within(panel).getByText("Written by Margince")).toBeInTheDocument();
  });

  it("says a composition was assembled, and names no model", () => {
    // The same read degrades to a composition over the same records, and the
    // foot says so rather than naming a writer that did not write.
    const panel = card("deterministic");
    expect(
      within(panel).getByText("Compiled from CRM records"),
    ).toBeInTheDocument();
    expect(within(panel).queryByText("Written by Margince")).toBeNull();
  });

  it("names no writer and stamps nothing when there is no brief", () => {
    // An empty brief falls back to the recorded profile, without AI provenance.
    const panel = card(undefined);
    expect(within(panel).queryByText(/Margince|Assembled/)).toBeNull();
    expect(
      screen.getByRole("heading", { name: "About this contact" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Dana Buyer")).toBeInTheDocument();
  });
});
