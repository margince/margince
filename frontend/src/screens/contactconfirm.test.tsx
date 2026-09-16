/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ContactConfirmCallout } from "./contactconfirm";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

type Contact360 = components["schemas"]["Contact360"];
type ProfileField = components["schemas"]["ContactProfileField"];

// The overview's ask about the enrichment pass itself: a handful of fields a
// machine read and nobody has ruled on yet. Rendered only while at least one
// is pending: a reader who has already confirmed or corrected everything the
// pass read owes this card nothing more, and a card that stayed up anyway
// would be asking twice for one answer.

const CONTACT = "3f7c1a90-0000-4000-8000-00000000c001";

function field(over: Partial<ProfileField>): ProfileField {
  return {
    field: "phone",
    value: "+49 30 1234",
    evidence_snippet: "Call me on +49 30 1234",
    source: "capture_enrich",
    captured_by: "agent:enrich",
    captured_at: "2026-08-13T09:00:00Z",
    ...over,
  };
}

function view(fields: ProfileField[]): Contact360 {
  return {
    as_of: "2026-08-20T09:00:00Z",
    contact: {
      id: CONTACT,
      full_name: "Dana Buyer",
      source: "manual",
      captured_by: "human:u-1",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-08-20T09:00:00Z",
    },
    sections_omitted: [],
    commercial: { role: null, committee: [] },
    claims: [],
    profile_fields: fields,
  };
}

beforeEach(() => {
  installFetchStub({ "GET /me": meRoute({}) });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the overview asks about what the enrichment pass has not been ruled on", () => {
  it("counts the unverdicted fields and offers to review them", () => {
    const { container } = render(
      <StoryProviders>
        <ContactConfirmCallout
          view={view([
            field({
              field: "phone",
              captured_at: "2026-08-13T09:00:00Z",
            }),
            field({
              field: "role",
              value: "VP Sales",
              captured_at: "2026-08-20T09:00:00Z",
            }),
          ])}
        />
      </StoryProviders>,
    );
    expect(screen.getByText("2 details to confirm")).toBeInTheDocument();
    expect(
      screen.getByText("Phone and Role were read from their emails on 20 Aug."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review" })).toBeInTheDocument();
    expect(container.querySelector(".callout-ai")).not.toBeNull();
  });

  it("renders nothing once every field carries a verdict", () => {
    const { container } = render(
      <StoryProviders>
        <ContactConfirmCallout
          view={view([
            field({ field: "phone", verdict: "confirmed" }),
            field({ field: "role", verdict: "corrected" }),
          ])}
        />
      </StoryProviders>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
