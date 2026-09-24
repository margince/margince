/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type BillingContact, BillingContactsPanel } from "./billingcontacts";
import { ContactBillingRoles } from "./contactbillingroles";

// The rule this panel turns on is the difference between WITHHELD and EMPTY,
// and it is the one that fails silently: both render as "no rows", and a card
// that shows "nobody is named" to a reader who simply lacks the grant states a
// fact about the account that nobody established.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const COMPANY = "o-1";

function render(
  ui: React.ReactNode,
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  return {
    qc,
    ...rtlRender(
      <QueryClientProvider client={qc}>
        <LocaleProvider>{ui}</LocaleProvider>
      </QueryClientProvider>,
    ),
  };
}

const PAT: BillingContact = {
  relationship_id: "r-1",
  contact_id: "c-1",
  full_name: "Pat Okafor",
  role: "recipient",
  email: "pat@acme.test",
};

it("renders each billing contact with their capacity and address", () => {
  render(<BillingContactsPanel contacts={[PAT]} companyId={COMPANY} />);
  expect(screen.getByText("Pat Okafor")).toBeTruthy();
  expect(screen.getByText("Invoice recipient")).toBeTruthy();
  expect(screen.getByRole("link", { name: "pat@acme.test" })).toBeTruthy();
});

it("says so when a named recipient has nowhere to send the invoice", () => {
  render(
    <BillingContactsPanel
      contacts={[{ ...PAT, email: null }]}
      companyId={COMPANY}
    />,
  );
  // Stated rather than left blank: a missing address is the one thing about a
  // billing contact that stops the invoice arriving, and an empty line reads
  // as fine to somebody scanning the list.
  expect(screen.getByText("No email recorded")).toBeTruthy();
});

it("distinguishes nobody named from not allowed to see", () => {
  const { unmount } = render(
    <BillingContactsPanel contacts={[]} companyId={COMPANY} />,
  );
  // Empty is a real answer about the account and says so, because a paying
  // customer with no recipient on file is a gap somebody should close.
  expect(screen.getByText(/Nobody is named yet/)).toBeTruthy();
  unmount();

  // Undefined means the server withheld the section. The panel does not
  // appear at all — claiming "nobody is named" here would be a statement this
  // reader has no standing to make.
  render(<BillingContactsPanel contacts={undefined} companyId={COMPANY} />);
  expect(screen.queryByText(/Nobody is named yet/)).toBeNull();
  expect(screen.queryByText("Billing contacts")).toBeNull();
});

it("lists one contact's several capacities in invoice order", () => {
  render(
    <BillingContactsPanel
      companyId={COMPANY}
      contacts={[
        PAT,
        { ...PAT, relationship_id: "r-2", role: "approver" },
        { ...PAT, relationship_id: "r-3", role: "accounts_payable" },
      ]}
    />,
  );
  // A small customer's office manager is often all three, and each capacity is
  // its own row because each is its own edge the panel can change alone.
  expect(screen.getAllByText("Pat Okafor")).toHaveLength(3);
  expect(screen.getByText("Approves")).toBeTruthy();
  expect(screen.getByText("Accounts payable")).toBeTruthy();
});

it("shows nothing on a contact who handles nobody's invoices", () => {
  // Unlike the company panel, an empty list renders nothing: most contacts
  // handle no invoices, and an empty panel on every contact page in the
  // product would be noise stating the obvious.
  render(<ContactBillingRoles companies={[]} />);
  expect(screen.queryByText("Billing roles")).toBeNull();
});

it("names the companies a contact bills for", () => {
  render(
    <ContactBillingRoles
      companies={[
        {
          relationship_id: "r-9",
          company_id: "o-1",
          company_name: "Acme",
          role: "approver",
        },
      ]}
    />,
  );
  expect(screen.getByText("Billing roles")).toBeTruthy();
  expect(screen.getByText("Acme")).toBeTruthy();
  expect(screen.getByText("Approves")).toBeTruthy();
});

// The three verbs write a RELATIONSHIP, so the panel asks for that grant and
// for a seat that may mutate at all. `user` is required on MeResponse — useMe
// reads a payload without it as a server answering garbage, which would fail
// every grant check for a reason that has nothing to do with grants.
const GRANTED = {
  user: { id: "u-1", email: "rep@example.com" },
  authorization: {
    seat_type: "full",
    objects: { relationship: { create: true, update: true, delete: true } },
  },
};

type Seen = {
  method: string;
  url: string;
  body?: unknown;
  ifMatch?: string | null;
};

function stubFetch(
  seen: Seen[],
  opts: { version?: number; me?: unknown } = {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const entry: Seen = { method: req.method, url: req.url };
      if (req.method !== "GET" && req.method !== "DELETE") {
        entry.body = await req.clone().json();
      }
      entry.ifMatch = req.headers.get("If-Match");
      seen.push(entry);
      if (req.method === "GET" && req.url.includes("/me")) {
        return new Response(JSON.stringify(opts.me ?? GRANTED), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      // The version lookup the PATCH path makes before it can pin its write.
      if (req.method === "GET" && req.url.includes("/relationships")) {
        return new Response(
          JSON.stringify({
            data:
              opts.version === undefined
                ? []
                : [{ id: "r-1", version: opts.version }],
            page: { next_cursor: null },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response(JSON.stringify({ id: "r-9" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

// settled waits for the grant snapshot to have ARRIVED before an absence is
// read as a refusal.
//
// The contacts arrive as a PROP, so the panel renders their names immediately
// and every verb is absent until /me answers. An absence asserted against that
// instant is true of a panel that has not decided yet, which is also true of a
// panel whose gate was deleted — so the assertion cannot tell the two apart.
// Waiting on the request the gate reads is what makes "no verbs" mean refused.
async function settled(seen: Seen[]) {
  await waitFor(() =>
    expect(seen.some((entry) => entry.url.includes("/me"))).toBe(true),
  );
}

it("offers no verbs at all on a company this reader cannot write", async () => {
  const seen: Seen[] = [];
  stubFetch(seen);
  render(
    <BillingContactsPanel contacts={[PAT]} companyId={COMPANY} readOnly />,
  );
  // The seat here is fully granted: what refuses is the archived company, so
  // the verbs would be drawn if readOnly were ignored. That only means
  // something once the grant has landed.
  await settled(seen);
  // Who is invoiced still renders: that is a fact a read-only reader is
  // entitled to. What goes is every way to change it.
  expect(screen.getByText("Pat Okafor")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Add contact" })).toBeNull();
  expect(
    screen.queryByRole("button", { name: /^Change the capacity/ }),
  ).toBeNull();
});

it("offers no verbs to a reader without the relationship grant", async () => {
  // The company is perfectly writable and NOT archived — what is missing is
  // the grant to write a relationship. Before this the panel drew three
  // enabled buttons from the archive flag alone, and a read-seat colleague
  // learned they could not use them from a refusal after submitting.
  const seen: Seen[] = [];
  stubFetch(seen, {
    me: {
      user: { id: "u-2", email: "reader@example.com" },
      authorization: { seat_type: "read", objects: {} },
    },
  });
  render(<BillingContactsPanel contacts={[PAT]} companyId={COMPANY} />);
  await settled(seen);
  expect(screen.getByText("Pat Okafor")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Add contact" })).toBeNull();
  expect(
    screen.queryByRole("button", { name: /^Change the capacity/ }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: /^Take Pat Okafor off/ }),
  ).toBeNull();
});

// The seat the create gate let through, and the commonest one in the product.
//
// `rep` holds writeNoDelete on relationship: it may name a billing contact and
// change their capacity, and the server refuses it Remove. Sharing one gate
// across the three verbs drew a button that answered "you do not have
// permission" after the press — the same defect the read-seat case above fixed,
// surviving for the seat that actually writes.
it("offers naming and changing but not Remove to a seat that cannot delete", async () => {
  stubFetch([], {
    me: {
      user: { id: "u-3", email: "rep@example.com" },
      authorization: {
        seat_type: "full",
        objects: { relationship: { create: true, update: true } },
      },
    },
  });
  render(<BillingContactsPanel contacts={[PAT]} companyId={COMPANY} />);
  expect(await screen.findByText("Pat Okafor")).toBeTruthy();
  // AWAITED on a verb, not on the contact's name: the contacts arrive as a
  // PROP and render before /me has answered, so anything awaited on them is
  // true while the grant snapshot is still in flight — which makes an
  // absence assertion pass against a panel that simply has not decided yet.
  expect(
    await screen.findByRole("button", { name: "Add contact" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: /^Change the capacity/ }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: /^Take Pat Okafor off/ }),
  ).toBeNull();
});

it("takes a billing contact off the account, pinned to its own version", async () => {
  const seen: Seen[] = [];
  stubFetch(seen, { version: 5 });
  render(<BillingContactsPanel contacts={[PAT]} companyId={COMPANY} />);
  // `findBy`, not `getBy`: the panel asks for its relationship grant over the
  // wire, so the verbs appear one tick after the first render.
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Take Pat Okafor off this account's invoices",
    }),
  );
  const del = seen.find((s) => s.method === "DELETE");
  expect(del?.url).toContain("/relationships/r-1");
  // Taking somebody off an account is exactly the write that should LOSE a
  // race with a colleague who just changed their capacity, rather than
  // silently winning it.
  expect(del?.ifMatch).toBe("5");
});

it("pins a capacity change to the edge's own version", async () => {
  const seen: Seen[] = [];
  // The finance summary projects a billing contact WITHOUT its version — it is
  // a reading, not the row — so the write has to resolve one first. Unpinned
  // it would land straight over whatever changed underneath.
  stubFetch(seen, { version: 7 });
  render(<BillingContactsPanel contacts={[PAT]} companyId={COMPANY} />);
  // `findBy`, not `getBy`: the panel asks for its relationship grant over the
  // wire, so the verbs appear one tick after the first render.
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Change the capacity Pat Okafor holds",
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Save change" }));
  const patch = seen.find((s) => s.method === "PATCH");
  expect(patch?.ifMatch).toBe("7");
});

it("narrows the version lookup to the one contact", async () => {
  const seen: Seen[] = [];
  stubFetch(seen, { version: 5 });
  render(<BillingContactsPanel contacts={[PAT]} companyId={COMPANY} />);
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Take Pat Okafor off this account's invoices",
    }),
  );
  // `/relationships` answers one page at a time. Asked for a whole company's
  // billing edges, a company with more of them than fit on a page would leave
  // every row past the first page unresolvable, and the reader would be told
  // the edge could not be read back however often they reloaded.
  const lookup = seen.find(
    (s) => s.method === "GET" && s.url.includes("/relationships"),
  );
  expect(lookup?.url).toContain("contact_id=c-1");
  expect(lookup?.url).toContain("company_id=o-1");
});

it("refreshes both the Finance and the Contacts projections after a write", async () => {
  // The panel is read from two projections — the finance summary the Finance
  // tab shows, and the Company360 the Contacts tab reads. A write that
  // refreshed only the first would leave whichever tab the reader is not on
  // showing the edit beside its own stale list.
  const seen: Seen[] = [];
  stubFetch(seen, { version: 5 });
  const { qc } = render(
    <BillingContactsPanel contacts={[PAT]} companyId={COMPANY} />,
  );
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Take Pat Okafor off this account's invoices",
    }),
  );
  await waitFor(() => {
    const keys = invalidate.mock.calls.map((call) => call[0]?.queryKey);
    expect(keys).toContainEqual(["finance-summary", COMPANY]);
    expect(keys).toContainEqual(["company360", COMPANY]);
  });
});

it("says so rather than writing unpinned when the edge cannot be read back", async () => {
  const seen: Seen[] = [];
  // The list the lookup scopes by can legitimately come back without the row:
  // a narrower read scope, a paged response, an edge somebody archived. A
  // write sent anyway would have no precondition at all.
  stubFetch(seen, { version: undefined });
  render(<BillingContactsPanel contacts={[PAT]} companyId={COMPANY} />);
  // `findBy`, not `getBy`: the panel asks for its relationship grant over the
  // wire, so the verbs appear one tick after the first render.
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Change the capacity Pat Okafor holds",
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Save change" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(seen.some((s) => s.method === "PATCH")).toBe(false);
});
