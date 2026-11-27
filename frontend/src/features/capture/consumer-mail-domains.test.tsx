/** @vitest-environment happy-dom */
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
import { SEARCH_DEBOUNCE_MS } from "../../components/listquery";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { steppedClock } from "../../testing/steppedclock";
import { ConsumerMailDomainsCard } from "./consumer-mail-domains";

// The workspace's own consumer-mail list. The server's write split: adding a
// NEW `extra` entry admits on capture_settings create OR update (the upsert
// pair), while the `never` carve-out, kind overwrites and removal demand
// update — so a fixture granting capture_settings delete, or any grant on a
// mail-shaped object, must leave every control inert.
const CAPTURE_EDITOR: GrantSpec = { capture_settings: ["read", "update"] };

// The shipped baseline answers only a search for "gm", so a list on screen can
// only have come from the settled search reaching the wire.
const BASELINE_GM = ["gmail.com", "gmx.de", "gmx.net"];

function backend(allow: GrantSpec) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    let body: unknown;
    if (url.endsWith("/v1/me")) {
      body = meFixture({ allow });
    } else if (url.includes("/consumer-mail-baseline")) {
      const searched = new URL(url, "https://test.local").searchParams.get("q");
      body =
        searched === "gm"
          ? { data: BASELINE_GM, matched: 40, total: 8758 }
          : { data: [], matched: 0, total: 8758 };
    } else {
      // id is required by the ConsumerMailDomain contract and is what remove
      // sends; a fixture without it would let a broken remove path call
      // mutate(undefined) and still pass.
      body = { data: [{ id: "cmd-1", domain: "gmx.test", kind: "extra" }] };
    }
    return new Response(JSON.stringify(body), {
      headers: { "Content-Type": "application/json" },
    });
  });
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

// The card's create verb lives in the panel header now, and it names the whole
// act ("Add domain") while the dialog it opens keeps the bare "Add" — so
// naming it by the catalog key is what stops this query matching the submit
// button instead.
const addButton = () =>
  screen.getByRole("button", {
    name: en["consumerMail.addOpen"],
  }) as HTMLButtonElement;

// The per-row remove control is icon-only, so its name says which domain it
// takes back: a list of identical "Remove" buttons names nothing.
const removeButton = () =>
  screen.getByRole("button", { name: "Remove gmx.test" }) as HTMLButtonElement;

describe("ConsumerMailDomainsCard", () => {
  it("enables add and remove on capture_settings:update", async () => {
    vi.stubGlobal("fetch", backend(CAPTURE_EDITOR));
    render(
      <Providers>
        <ConsumerMailDomainsCard />
      </Providers>,
    );

    await waitFor(() => expect(screen.getByText("gmx.test")).toBeTruthy());
    expect(addButton().disabled).toBe(false);
    expect(removeButton().disabled).toBe(false);
  });

  it("leaves both inert without the update grant", async () => {
    vi.stubGlobal("fetch", backend({ capture_settings: ["read"] }));
    render(
      <Providers>
        <ConsumerMailDomainsCard />
      </Providers>,
    );

    await waitFor(() => expect(screen.getByText("gmx.test")).toBeTruthy());
    // Both, not just Add: the two share one grant, so a change that decoupled
    // them would otherwise pass.
    expect(addButton().disabled).toBe(true);
    expect(removeButton().disabled).toBe(true);
  });

  // The rep posture: create without update adds a NEW consumer domain, and
  // nothing else — remove stays inert, mirroring the server's split.
  it("enables add but not remove on capture_settings:create alone", async () => {
    vi.stubGlobal("fetch", backend({ capture_settings: ["read", "create"] }));
    render(
      <Providers>
        <ConsumerMailDomainsCard />
      </Providers>,
    );

    await waitFor(() => expect(screen.getByText("gmx.test")).toBeTruthy());
    expect(addButton().disabled).toBe(false);
    expect(removeButton().disabled).toBe(true);
  });

  // Removal is an UPDATE. A principal holding delete but not update must not
  // reach the remove control — that is the trap this binding exists to avoid.
  it("does not mistake capture_settings:delete for permission to remove", async () => {
    vi.stubGlobal("fetch", backend({ capture_settings: ["read", "delete"] }));
    render(
      <Providers>
        <ConsumerMailDomainsCard />
      </Providers>,
    );

    await waitFor(() => expect(screen.getByText("gmx.test")).toBeTruthy());
    // Both, not just Add: the two share one grant, so a change that decoupled
    // them would otherwise pass.
    expect(addButton().disabled).toBe(true);
    expect(removeButton().disabled).toBe(true);
  });

  // Any role may search the shipped list, so the reader here holds read alone:
  // the lookup answers the question "does the baseline already cover this?"
  // before anybody decides whether an entry is needed.
  it("lists each shipped domain the settled search matched, and says how many more there are", async () => {
    const user = steppedClock();
    vi.stubGlobal("fetch", backend({ capture_settings: ["read"] }));
    render(
      <Providers>
        <ConsumerMailDomainsCard />
      </Providers>,
    );

    await user.type(
      await screen.findByLabelText(en["consumerMail.baselineSearchLabel"]),
      "gm",
    );
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);

    const list = await screen.findByTestId("consumer-mail-baseline-list");
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(BASELINE_GM);
    // Three shown of forty matched: the list is a first page, and says so
    // rather than reading as every consumer domain that starts with "gm".
    expect(screen.getByText("Showing the first 3 of 40 matches.")).toBeTruthy();
  });

  // The list refuses what is not consumer mail and asks about a customer.
  describe("adding a domain that is already something", () => {
    const COMPANY_QUESTION = {
      code: "domain_needs_confirmation",
      status: 409,
      detail: "demo.test is already on file",
      details: {
        domain: "demo.test",
        role: "company",
        company_name: "Demo GmbH",
      },
    };

    function addingWith(post: (body: unknown) => Response) {
      const base = backend(CAPTURE_EDITOR);
      const posts: unknown[] = [];
      vi.stubGlobal(
        "fetch",
        vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
          const request = input instanceof Request ? input : null;
          const method = request?.method ?? init?.method ?? "GET";
          if (method === "POST") {
            const text = request ? await request.text() : String(init?.body);
            posts.push(JSON.parse(text));
            return post(posts[posts.length - 1]);
          }
          return base(input);
        }),
      );
      return posts;
    }

    async function typeDomain(domain: string) {
      const user = userEvent.setup();
      render(
        <Providers>
          <ConsumerMailDomainsCard />
        </Providers>,
      );
      await waitFor(() => expect(screen.getByText("gmx.test")).toBeTruthy());
      await user.click(addButton());
      await user.type(screen.getByTestId("consumer-mail-domain-input"), domain);
      await user.click(
        within(screen.getByRole("dialog")).getByRole("button", {
          name: en["consumerMail.add"],
        }),
      );
      return user;
    }

    it("says the domain is required on Add, and sends nothing", async () => {
      const posts = addingWith(() => json({}, 201));
      const user = userEvent.setup();
      render(
        <Providers>
          <ConsumerMailDomainsCard />
        </Providers>,
      );
      await waitFor(() => expect(screen.getByText("gmx.test")).toBeTruthy());
      await user.click(addButton());
      await user.click(
        within(screen.getByRole("dialog")).getByRole("button", {
          name: en["consumerMail.add"],
        }),
      );
      expect(screen.getByText("Domain is required.")).toBeTruthy();
      expect(document.activeElement).toBe(
        screen.getByTestId("consumer-mail-domain-input"),
      );
      expect(posts).toHaveLength(0);
    });

    it("names the company and resends with confirm once the user says yes", async () => {
      const posts = addingWith((body) =>
        typeof body === "object" && body !== null && "confirm" in body
          ? json({ id: "cmd-2", domain: "demo.test", kind: "extra" }, 201)
          : json(COMPANY_QUESTION, 409),
      );
      const user = await typeDomain("demo.test");

      expect(
        await screen.findByText(
          "demo.test is the domain of Demo GmbH. Mail from it will no longer create a company. Add anyway?",
        ),
      ).toBeTruthy();
      await user.click(
        screen.getByRole("button", { name: en["captureValue.addAnyway"] }),
      );
      await waitFor(() => expect(posts).toHaveLength(2));
      expect(posts[1]).toMatchObject({ domain: "demo.test", confirm: true });
    });

    it("says a shipped domain is already there and offers no way around it", async () => {
      addingWith(() =>
        json(
          {
            code: "validation_error",
            status: 422,
            detail: "gmail.com is already on the shipped list",
            details: {
              errors: [
                {
                  field: "domain",
                  code: "already_on_shipped_list",
                  message: "gmail.com is already on the shipped list",
                },
              ],
            },
          },
          422,
        ),
      );
      await typeDomain("gmail.com");

      expect(
        await screen.findByText("gmail.com is already on the shipped list."),
      ).toBeTruthy();
      expect(
        screen.queryByRole("button", { name: en["captureValue.addAnyway"] }),
      ).toBeNull();
    });

    it("says the own domain cannot be listed as consumer mail", async () => {
      addingWith(() =>
        json(
          {
            code: "validation_error",
            status: 422,
            detail: "ours.test is your own domain",
            details: {
              errors: [
                {
                  field: "domain",
                  code: "own_domain",
                  message: "ours.test is your own domain",
                },
              ],
            },
          },
          422,
        ),
      );
      await typeDomain("ours.test");

      expect(
        await screen.findByText("ours.test is your own domain."),
      ).toBeTruthy();
    });
  });
});
