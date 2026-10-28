// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { ToastProvider, ToastRegion } from "../../../design-system/toast";
import { LocaleProvider } from "../../../i18n";
import { type GrantSpec, meFixture } from "../../../testing/mefixture";
import { CompaniesScreen } from "../companies";
import { CompanyContactsList } from "./contacts";
import { DomainContactsArrival, offerDomainContacts } from "./domaincontacts";
import {
  domainCandidates,
  fiftyDomainCandidates,
} from "./domaincontacts.fixtures";

type CandidateList = components["schemas"]["CompanyLinkCandidateList"];

const COMPANY_ID = "3f6b1c2e-8a47-4d19-b0e5-7c2a9d4e6f81";

const candidates = domainCandidates;

const ALL_GRANTS: GrantSpec = {
  company: ["read", "create", "update"],
  contact: ["read", "update"],
  relationship: ["read", "create"],
};

type Sent = { path: string; body: unknown; key: string | null };

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function problem(status: number, extra: Record<string, unknown> = {}) {
  return json(
    { type: "about:blank", title: "Refused", status, ...extra },
    status,
  );
}

function stubApi({
  grants = ALL_GRANTS,
  lists = [{ data: candidates, truncated: false }],
  read,
  link = () => json({ data: [] }, 201),
}: {
  grants?: GrantSpec;
  // The candidate read answers the list for the link attempts made so far.
  lists?: CandidateList[];
  read?: () => Response;
  link?: () => Response;
} = {}) {
  const sent: Sent[] = [];
  const reads: string[] = [];
  const seen: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      seen.push(path);
      if (path === "/me") return json(meFixture({ allow: grants }));
      if (path === `/companies/${COMPANY_ID}/link-candidates`) {
        reads.push(path);
        if (read) return read();
        return json(lists[Math.min(sent.length, lists.length - 1)]);
      }
      if (path === `/companies/${COMPANY_ID}/link-contacts`) {
        sent.push({
          path,
          body: JSON.parse(await request.text()),
          key: request.headers.get("Idempotency-Key"),
        });
        return link();
      }
      if (request.method === "POST" && path === "/companies") {
        const body = JSON.parse(await request.text());
        return json(
          {
            id: COMPANY_ID,
            display_name: body.display_name,
            domains: body.domains ?? [],
            version: 1,
            source: "manual",
            captured_by: "human:u1",
            created_at: "2026-10-28T09:00:00Z",
            updated_at: "2026-10-28T09:00:00Z",
          },
          201,
        );
      }
      return json({ data: [], page: { has_more: false } });
    }),
  );
  return { sent, reads, seen };
}

function mount(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function createFromList(domain?: string) {
  const user = userEvent.setup();
  await user.click(await screen.findByTestId("new-record"));
  await user.type(
    screen.getByLabelText("Company name *"),
    "Nordlicht Logistik GmbH",
  );
  if (domain) {
    await user.click(screen.getByText("Add domain"));
    await user.type(screen.getByLabelText("Domain *"), domain);
  }
  await user.click(screen.getByRole("button", { name: "Create" }));
  return user;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("Contacts on a new company’s domain", () => {
  it("opens over the created company with nothing ticked and sends only the ticked contacts", async () => {
    const { sent } = stubApi();
    mount(
      <>
        <CompaniesScreen />
        <DomainContactsArrival companyId={COMPANY_ID} />
      </>,
    );
    const user = await createFromList("nordlicht-logistik.de");

    const dialog = await screen.findByRole("dialog");
    const ticks = within(dialog).getAllByRole("checkbox");
    expect(ticks).toHaveLength(3);
    for (const tick of ticks) expect(tick).not.toBeChecked();
    expect(
      within(dialog).getByText("Works at Hanse Spedition KG"),
    ).toBeVisible();
    expect(
      within(dialog).getByText("anja.petersen@nordlicht-logistik.de"),
    ).toBeVisible();
    expect(
      within(dialog).getByRole("button", { name: "Link contacts" }),
    ).toBeDisabled();

    await user.click(within(dialog).getByRole("checkbox", { name: /Anja/ }));
    await user.click(within(dialog).getByRole("checkbox", { name: /Mira/ }));
    await user.click(
      within(dialog).getByRole("button", { name: "Link 2 contacts" }),
    );

    expect(await screen.findByText("2 contacts linked")).toBeVisible();
    expect(sent).toHaveLength(1);
    expect(sent[0].body).toEqual({
      contact_ids: [candidates[0].contact_id, candidates[2].contact_id],
    });
    expect(sent[0].key).not.toBeNull();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("asks nothing when the created company has no domain", async () => {
    const { reads } = stubApi();
    mount(
      <>
        <CompaniesScreen />
        <DomainContactsArrival companyId={COMPANY_ID} />
      </>,
    );
    await createFromList();

    await waitFor(() =>
      expect(window.location.hash).toBe(`#/companies/${COMPANY_ID}`),
    );
    expect(reads).toHaveLength(0);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens no dialog when the domain matches nobody", async () => {
    const { reads } = stubApi({ lists: [{ data: [], truncated: false }] });
    offerDomainContacts({ id: COMPANY_ID, domains: [{}] });
    mount(<DomainContactsArrival companyId={COMPANY_ID} />);

    await waitFor(() => expect(reads).toHaveLength(1));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("does not offer a link the role may not make", async () => {
    const { reads, seen } = stubApi({
      grants: { company: ["read"], contact: ["read"], relationship: ["read"] },
    });
    offerDomainContacts({ id: COMPANY_ID, domains: [{}] });
    mount(<DomainContactsArrival companyId={COMPANY_ID} />);

    await waitFor(() => expect(seen).toContain("/me"));
    expect(reads).toHaveLength(0);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("The Contacts tab’s domain callout", () => {
  it("counts the unlinked contacts and opens the same dialog", async () => {
    stubApi();
    mount(<CompanyContactsList companyId={COMPANY_ID} />);
    const user = userEvent.setup();

    expect(
      await screen.findByText("3 contacts on this domain are not linked"),
    ).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Review contacts" }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getAllByRole("checkbox")).toHaveLength(3);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("says only the first 50 are listed when the server cut the list", async () => {
    stubApi({ lists: [{ data: fiftyDomainCandidates, truncated: true }] });
    mount(<CompanyContactsList companyId={COMPANY_ID} />);
    const user = userEvent.setup();

    expect(
      await screen.findByText(
        "More than 50 contacts on this domain are not linked",
      ),
    ).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Review contacts" }));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText("Only the first 50 contacts are listed."),
    ).toBeVisible();
  });

  it("refuses the verb with its reason when the role may not link", async () => {
    stubApi({
      grants: { company: ["read"], contact: ["read"], relationship: ["read"] },
    });
    mount(<CompanyContactsList companyId={COMPANY_ID} />);

    const review = await screen.findByRole("button", {
      name: "Review contacts",
    });
    expect(review).toBeDisabled();
    expect(
      screen.getByText(
        "Your role cannot link contacts to companies. Ask an admin.",
      ),
    ).toBeVisible();
  });

  it("shows nothing when the candidate read is refused", async () => {
    const { reads } = stubApi({ read: () => problem(403) });
    mount(<CompanyContactsList companyId={COMPANY_ID} />);

    await waitFor(() => expect(reads).toHaveLength(1));
    expect(
      screen.queryByRole("button", { name: "Review contacts" }),
    ).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("A refused link", () => {
  it("re-reads the candidates and says the list changed", async () => {
    const { reads, sent } = stubApi({
      lists: [
        { data: candidates, truncated: false },
        { data: candidates.slice(1), truncated: false },
      ],
      link: () =>
        problem(422, {
          code: "validation_error",
          details: {
            contact_id: candidates[0].contact_id,
            errors: [
              {
                field: "contact_ids",
                code: "not_a_link_candidate",
                message: "not a link candidate",
              },
            ],
          },
        }),
    });
    mount(<CompanyContactsList companyId={COMPANY_ID} />);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Review contacts" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("checkbox", { name: /Anja/ }));
    await user.click(within(dialog).getByRole("checkbox", { name: /Jonas/ }));
    await user.click(
      within(dialog).getByRole("button", { name: "Link 2 contacts" }),
    );

    expect(
      await within(dialog).findByText(
        "The list changed while it was open. Check the ticks and link again.",
      ),
    ).toBeVisible();
    await waitFor(() =>
      expect(within(dialog).queryByText("Anja Petersen")).toBeNull(),
    );
    expect(reads.length).toBeGreaterThanOrEqual(2);
    expect(
      within(dialog).getByRole("checkbox", { name: /Jonas/ }),
    ).toBeChecked();
    expect(
      within(dialog).getByRole("button", { name: "Link 1 contact" }),
    ).toBeEnabled();

    await user.click(
      within(dialog).getByRole("button", { name: "Link 1 contact" }),
    );
    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[1].key).not.toBe(sent[0].key);
  });

  it("names a refused write in words, not the raw response", async () => {
    stubApi({
      link: () => problem(403, { code: "permission_denied" }),
    });
    mount(<CompanyContactsList companyId={COMPANY_ID} />);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Review contacts" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("checkbox", { name: /Mira/ }));
    await user.click(
      within(dialog).getByRole("button", { name: "Link 1 contact" }),
    );

    expect(
      await within(dialog).findByText(/You do not have permission/),
    ).toBeVisible();
    expect(within(dialog).queryByText("Refused")).toBeNull();
  });
});
