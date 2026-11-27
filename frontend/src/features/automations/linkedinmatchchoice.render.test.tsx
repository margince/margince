/** @vitest-environment happy-dom */

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import type { Approval } from "../../lib/approvals.queries";
import { companyNamesAnswer } from "../../testing/companynames.testkit";
import type { GrantSpec } from "../../testing/mefixture";
import { recordNamesAnswer } from "../../testing/recordnames.testkit";
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

// The full company-list fixture remains available to list consumers.
function acmeNamed(): Response {
  return jsonResponse({
    data: [{ id: ACME, display_name: "Acme GmbH" }],
    next_cursor: null,
  });
}

function stubCompanyReferences(
  read: (url: string) => Response | Promise<Response> = (url) =>
    companyNamesAnswer(url, { [ACME]: "Acme GmbH" }) ??
    jsonResponse({ data: [] }),
) {
  const routed = globalThis.fetch;
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input instanceof Request ? input.url : String(input);
    const asked = new URL(url, "https://test.local");
    const method =
      input instanceof Request ? input.method : (init?.method ?? "GET");
    if (
      method === "GET" &&
      asked.pathname === "/v1/records/references" &&
      asked.searchParams.get("kind") === "company"
    ) {
      return Promise.resolve(read(url));
    }
    return routed(input, init);
  });
}

// Routes the reads the card makes and records what the accept sent.
function stubCard(
  allow: GrantSpec,
  company: () => Response | Promise<Response> = acmeNamed,
  reference?: (url: string) => Response | Promise<Response>,
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
  stubCompanyReferences(reference);
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
      screen.getByRole("checkbox", {
        name: englishCopy["approval.linkedin.useAsTitle"],
      }),
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
      await screen.findByRole("checkbox", {
        name: englishCopy["approval.linkedin.useAsTitle"],
      }),
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
        await screen.findByRole("checkbox", {
          name: englishCopy["approval.linkedin.useAsTitle"],
        }),
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
    stubCompanyReferences();
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
    const pending = () => new Promise<Response>(() => {});
    const sent = stubCard(mayEmploy, pending, pending);
    show(linkedInMatch());
    await user.click(
      await screen.findByRole("checkbox", {
        name: englishCopy["approval.linkedin.useAsTitle"],
      }),
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
    await screen.findByRole("checkbox", {
      name: englishCopy["approval.linkedin.useAsTitle"],
    });
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
      await screen.findByRole("checkbox", {
        name: englishCopy["approval.linkedin.useAsTitle"],
      }),
    );
    expect(
      screen.getByRole("checkbox", {
        name: englishCopy["approval.linkedin.useAsTitle"],
      }),
    ).not.toBeChecked();
    view.rerender(
      <StoryProviders>
        <ApprovalRow approval={{ ...linkedInMatch(), id: "ap-other" }} />
      </StoryProviders>,
    );
    expect(
      await screen.findByRole("checkbox", {
        name: englishCopy["approval.linkedin.useAsTitle"],
      }),
    ).toBeChecked();
  });

  it("offers no editor for the export's strings", async () => {
    stubCard(mayEmploy);
    show(linkedInMatch());
    await screen.findByRole("button", { name: "Approve" });
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
  });
});

const CANDIDATE = "77777777-7777-4777-8777-777777777777";

function vcardCreate(change: Record<string, unknown> = {}): Approval {
  return {
    id: "ap-card",
    kind: "vcard_create",
    status: "pending",
    summary: "Anna Weber resembles an existing contact",
    proposed_by: "human:importer",
    proposed_change: {
      entry: { full_name: "Anna Weber" },
      name: "Anna Weber",
      candidate_contact_id: CANDIDATE,
      merge_into_candidate: false,
      ...change,
    },
    created_at: "2026-10-20T09:00:00Z",
  } as Approval;
}

function stubCardReview(sees: boolean) {
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ contact: ["create", "update"] }),
    "GET /records/references": (_body: unknown, url: URL) =>
      recordNamesAnswer(
        url,
        sees ? { contact: { [CANDIDATE]: "Anna W." } } : {},
      ) ?? jsonResponse({ data: [] }),
    "POST /approvals/ap-card/approve": (body) => {
      sent.push(body);
      return jsonResponse({ id: "ap-card", status: "approved" });
    },
  });
  return sent;
}

describe("the vCard review's merge box", () => {
  it("keeps the card separate, with no edit, when the box is left alone", async () => {
    const user = userEvent.setup();
    const sent = stubCardReview(true);
    show(vcardCreate());
    expect(
      await screen.findByRole("checkbox", {
        name: "Merge into Anna W. instead of creating a new contact",
      }),
    ).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toBeNull();
  });

  it("sends the whole staged change with the merge choice when the box is ticked", async () => {
    const user = userEvent.setup();
    const sent = stubCardReview(true);
    const approval = vcardCreate();
    show(approval);
    await user.click(
      await screen.findByRole("checkbox", { name: /Merge into Anna W\./ }),
    );
    await user.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual({
      edited_payload: {
        ...approval.proposed_change,
        merge_into_candidate: true,
      },
    });
  });

  it("offers no box for a contact this reader cannot see", async () => {
    stubCardReview(false);
    show(vcardCreate());
    await screen.findByRole("button", { name: "Approve" });
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("offers no box on a card staged before the choice existed", async () => {
    stubCardReview(true);
    show(vcardCreate({ merge_into_candidate: undefined }));
    await screen.findByRole("button", { name: "Approve" });
    expect(screen.queryByRole("checkbox")).toBeNull();
  });
});
