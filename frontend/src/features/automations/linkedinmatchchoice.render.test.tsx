/** @vitest-environment happy-dom */

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Approval } from "../../lib/approvals.queries";
import type { GrantSpec } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ApprovalRow } from "./approvalrow";

// The boxes on a LinkedIn match card, and the edit an accept sends.

const ACME = "33333333-3333-4333-8333-333333333333";
const CONTACT = "44444444-4444-4444-8444-444444444444";

function linkedInMatch(change: Record<string, unknown> = {}): Approval {
  return {
    id: "ap-li",
    kind: "linkedin_match",
    status: "pending",
    summary: "Andreas Müller at Acme GmbH looks like Andreas Muller",
    proposed_by: "system:linkedin",
    proposed_change: {
      connection_id: "55555555-5555-4555-8555-555555555555",
      contact_id: CONTACT,
      connection_name: "Andreas Müller",
      connection_company: "Acme GmbH",
      connection_position: "Head of Sales",
      contact_name: "Andreas Muller",
      employer_company_id: ACME,
      use_as_title: true,
      use_as_employer: true,
      ...change,
    },
    created_at: "2026-10-20T09:00:00Z",
    target_entity_type: "contact",
    target_entity_id: CONTACT,
  } as Approval;
}

// The batched company-name read, naming Acme; any other id asked is one the
// reader may not see, which the list answers by leaving it out.
function acmeNamed(): Response {
  return jsonResponse({
    data: [{ id: ACME, display_name: "Acme GmbH" }],
    next_cursor: null,
  });
}

// Routes the reads the card makes and records what the accept sent.
function stubCard(
  allow: GrantSpec,
  company: () => Response | Promise<Response> = acmeNamed,
) {
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute(allow),
    "GET /companies": company,
    [`GET /contacts/${CONTACT}`]: () =>
      jsonResponse({ id: CONTACT, full_name: "Andreas Muller" }),
    "POST /approvals/ap-li/approve": (body) => {
      sent.push(body);
      return jsonResponse({ id: "ap-li", status: "approved" });
    },
  });
  return sent;
}

function show(approval: Approval) {
  return render(
    <StoryProviders>
      <ApprovalRow approval={approval} />
    </StoryProviders>,
  );
}

const mayEmploy: GrantSpec = {
  relationship: ["create", "update"],
  company: ["read"],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the LinkedIn match boxes", () => {
  it("shows both boxes as staged and sends no edit when they are left alone", async () => {
    const user = userEvent.setup();
    const sent = stubCard(mayEmploy);
    show(linkedInMatch());
    expect(
      await screen.findByRole("checkbox", {
        name: "Use Acme GmbH as employer",
      }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: "Use as job title" }),
    ).toBeChecked();
    await user.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toBeNull();
  });

  it("sends the whole staged change with the box the reader unticked", async () => {
    const user = userEvent.setup();
    const sent = stubCard(mayEmploy);
    const approval = linkedInMatch();
    show(approval);
    await user.click(
      await screen.findByRole("checkbox", { name: "Use as job title" }),
    );
    await user.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual({
      edited_payload: { ...approval.proposed_change, use_as_title: false },
    });
  });

  it.each<[string, GrantSpec]>([
    ["no grant on employments", { company: ["read"] }],
    [
      "create without update, which a promote needs",
      { relationship: ["create"], company: ["read"] },
    ],
    [
      "update without create, which a new employment needs",
      { relationship: ["update"], company: ["read"] },
    ],
  ])(
    "hides the employer box from a reader with %s, and unticks it",
    async (_, allow) => {
      const user = userEvent.setup();
      const sent = stubCard(allow);
      show(linkedInMatch());
      expect(
        await screen.findByRole("checkbox", { name: "Use as job title" }),
      ).toBeChecked();
      expect(
        screen.queryByRole("checkbox", { name: /as employer/ }),
      ).toBeNull();
      await user.click(screen.getByRole("button", { name: "Approve" }));
      await waitFor(() => expect(sent).toHaveLength(1));
      expect(sent[0]).toMatchObject({
        edited_payload: { use_as_title: true, use_as_employer: false },
      });
    },
  );

  it("hides the employer box when the reader cannot see the company", async () => {
    const hidden = "66666666-6666-4666-8666-666666666666";
    installFetchStub({
      "GET /me": meRoute(mayEmploy),
      "GET /companies": acmeNamed,
    });
    const asked: string[] = [];
    const routed = globalThis.fetch;
    vi.stubGlobal("fetch", (request: Request) => {
      asked.push(...new URL(request.url).searchParams.getAll("id"));
      return routed(request);
    });
    const view = show(linkedInMatch());
    // The grants have arrived once a visible company shows its box.
    await screen.findByRole("checkbox", { name: "Use Acme GmbH as employer" });
    view.rerender(
      <StoryProviders>
        <ApprovalRow
          approval={{
            ...linkedInMatch({ employer_company_id: hidden }),
            id: "ap-hidden",
          }}
        />
      </StoryProviders>,
    );
    await waitFor(() => expect(asked).toContain(hidden));
    await waitFor(() =>
      expect(
        screen.queryByRole("checkbox", { name: /as employer/ }),
      ).toBeNull(),
    );
  });

  it("keeps the staged employer box when accepted before the company has loaded", async () => {
    const user = userEvent.setup();
    const sent = stubCard(mayEmploy, () => new Promise<Response>(() => {}));
    show(linkedInMatch());
    await user.click(
      await screen.findByRole("checkbox", { name: "Use as job title" }),
    );
    await user.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toMatchObject({
      edited_payload: { use_as_title: false, use_as_employer: true },
    });
  });

  it("offers no employer when the card names no company", async () => {
    stubCard(mayEmploy);
    show(
      linkedInMatch({ employer_company_id: undefined, use_as_employer: false }),
    );
    await screen.findByRole("checkbox", { name: "Use as job title" });
    expect(screen.queryByRole("checkbox", { name: /as employer/ })).toBeNull();
  });

  it("shows no boxes on a card staged before they existed", async () => {
    stubCard(mayEmploy);
    show(
      linkedInMatch({ use_as_title: undefined, use_as_employer: undefined }),
    );
    await screen.findByRole("button", { name: "Approve" });
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("starts a row handed another card from that card's staged boxes", async () => {
    const user = userEvent.setup();
    stubCard(mayEmploy);
    const view = show(linkedInMatch());
    await user.click(
      await screen.findByRole("checkbox", { name: "Use as job title" }),
    );
    expect(
      screen.getByRole("checkbox", { name: "Use as job title" }),
    ).not.toBeChecked();
    view.rerender(
      <StoryProviders>
        <ApprovalRow approval={{ ...linkedInMatch(), id: "ap-other" }} />
      </StoryProviders>,
    );
    expect(
      await screen.findByRole("checkbox", { name: "Use as job title" }),
    ).toBeChecked();
  });

  it("offers no editor for the export's strings", async () => {
    stubCard(mayEmploy);
    show(linkedInMatch());
    await screen.findByRole("button", { name: "Approve" });
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
  });
});
