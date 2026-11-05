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
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { workEmailAddress } from "./contactformfields";
import { NewContactAction } from "./newcontact";

const NORDLICHT = {
  id: "7c4b2e90-1d3a-4f58-b6e2-9a0c5d8f1e23",
  display_name: "Nordlicht Logistik AG",
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const emptyPage = { data: [], page: { next_cursor: null, has_more: false } };

// The address read answers as the server does: Nordlicht's domain covers its
// subdomains, and consumer mail never answers. `createAnswer` answers the
// contact write; `addressAnswer` replaces the address read.
function stubApi(
  createAnswer = () => json({ id: "p-new" }, 201),
  addressAnswer?: () => Response,
) {
  const posted: unknown[] = [];
  const addressLookups: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (request.method === "POST" && url.pathname.endsWith("/contacts")) {
        posted.push(JSON.parse(await request.text()));
        return createAnswer();
      }
      if (url.pathname.endsWith("/me")) {
        return json(meFixture({ allow: { contact: ["read", "create"] } }));
      }
      if (url.pathname.endsWith("/companies/for-address")) {
        const email = url.searchParams.get("email") ?? "";
        addressLookups.push(email);
        if (addressAnswer) return addressAnswer();
        return json(
          /@(.+\.)?nordlicht-logistik\.de$/.test(email)
            ? {
                company: {
                  company_id: NORDLICHT.id,
                  company_name: NORDLICHT.display_name,
                },
              }
            : {},
        );
      }
      if (url.pathname.endsWith("/companies")) {
        const q = (url.searchParams.get("q") ?? "").toLowerCase();
        return json({
          ...emptyPage,
          data: NORDLICHT.display_name.toLowerCase().includes(q)
            ? [NORDLICHT]
            : [],
        });
      }
      return json(emptyPage);
    }),
  );
  return { posted, addressLookups };
}

async function openNewContact(
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <NewContactAction />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(screen.getByTestId("new-record"));
  await user.type(await screen.findByLabelText("Full name *"), "Dana Buyer");
  return user;
}

async function typeWorkEmail(
  user: ReturnType<typeof userEvent.setup>,
  email: string,
) {
  await user.click(screen.getByText("Add email"));
  await user.type(screen.getByLabelText("Email *"), email);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("New contact’s Company field", () => {
  it("sends the picked company by id", async () => {
    const { posted } = stubApi();
    const user = await openNewContact();

    await user.type(screen.getByRole("combobox", { name: "Company" }), "Nord");
    await user.click(
      await within(await screen.findByRole("listbox")).findByRole("option", {
        name: "Nordlicht Logistik AG",
      }),
    );
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ company_id: NORDLICHT.id });
    expect(posted[0]).not.toHaveProperty("company_name");
  });

  it("sends a typed name as a company to create", async () => {
    const { posted } = stubApi();
    const user = await openNewContact();

    await user.type(
      screen.getByRole("combobox", { name: "Company" }),
      "Brandt Automotive GmbH",
    );
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ company_name: "Brandt Automotive GmbH" });
    expect(posted[0]).not.toHaveProperty("company_id");
  });

  it("marks the companies list stale once the contact is created", async () => {
    const { posted } = stubApi();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const companiesList = ["companies", {}];
    client.setQueryData(companiesList, emptyPage);
    const user = await openNewContact(client);

    await user.type(
      screen.getByRole("combobox", { name: "Company" }),
      "Brandt Automotive GmbH",
    );
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    await waitFor(() =>
      expect(client.getQueryState(companiesList)?.isInvalidated).toBe(true),
    );
  });

  it("picks the company that holds a typed work email’s domain", async () => {
    const { posted } = stubApi();
    const user = await openNewContact();

    await typeWorkEmail(user, "dana.buyer@nordlicht-logistik.de");

    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Company" })).toHaveValue(
        "Nordlicht Logistik AG",
      ),
    );
    expect(
      screen.getByText(
        "Matched to an existing company by the work email domain.",
      ),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ company_id: NORDLICHT.id });
  });

  it("sends the typed address and prefills the company that covers its subdomain", async () => {
    const { posted, addressLookups } = stubApi();
    const user = await openNewContact();

    await typeWorkEmail(user, "dana.buyer@eu.nordlicht-logistik.de");

    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Company" })).toHaveValue(
        "Nordlicht Logistik AG",
      ),
    );
    expect(addressLookups.at(-1)).toBe("dana.buyer@eu.nordlicht-logistik.de");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ company_id: NORDLICHT.id });
  });

  it("never prefills from consumer mail", async () => {
    const { posted, addressLookups } = stubApi();
    const user = await openNewContact();

    await typeWorkEmail(user, "dana.buyer@gmail.com");
    await waitFor(() =>
      expect(addressLookups).toContain("dana.buyer@gmail.com"),
    );
    expect(screen.getByRole("combobox", { name: "Company" })).toHaveValue("");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).not.toHaveProperty("company_id");
    expect(posted[0]).not.toHaveProperty("company_name");
  });

  it("asks nothing for an address still being typed", () => {
    const typed = (email: string) =>
      workEmailAddress({ emails: [{ email, email_type: "work" }] });

    expect(typed("dana@")).toBeUndefined();
    expect(typed("dana@nordlicht-logistik")).toBeUndefined();
    expect(typed(" dana@nordlicht-logistik.de ")).toBe(
      "dana@nordlicht-logistik.de",
    );
  });

  it("prefills nothing and shows no error when the read is refused", async () => {
    const { posted, addressLookups } = stubApi(undefined, () =>
      json({ type: "about:blank", title: "Forbidden", status: 403 }, 403),
    );
    const user = await openNewContact();

    await typeWorkEmail(user, "dana.buyer@nordlicht-logistik.de");
    await waitFor(() => expect(addressLookups).toHaveLength(1));
    const box = screen.getByRole("combobox", { name: "Company" });
    expect(box).toHaveValue("");
    expect(screen.queryByRole("alert")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).not.toHaveProperty("company_id");
    expect(screen.queryByText("Forbidden")).toBeNull();
  });

  it("never replaces a company the reader typed", async () => {
    const { posted, addressLookups } = stubApi();
    const user = await openNewContact();

    await user.type(
      screen.getByRole("combobox", { name: "Company" }),
      "Brandt Automotive GmbH",
    );
    await typeWorkEmail(user, "dana.buyer@nordlicht-logistik.de");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(addressLookups).toEqual([]);
    expect(posted[0]).toMatchObject({ company_name: "Brandt Automotive GmbH" });
    expect(posted[0]).not.toHaveProperty("company_id");
  });

  it("shows a refused company beside the Company field", async () => {
    stubApi(() =>
      json(
        {
          type: "about:blank",
          title: "Validation failed",
          status: 422,
          code: "validation_error",
          details: {
            errors: [
              {
                field: "company_id",
                code: "reference_not_found",
                message: "company_id does not name a company you can read",
              },
            ],
          },
        },
        422,
      ),
    );
    const user = await openNewContact();

    await user.type(screen.getByRole("combobox", { name: "Company" }), "Nord");
    await user.click(
      await within(await screen.findByRole("listbox")).findByRole("option", {
        name: "Nordlicht Logistik AG",
      }),
    );
    await user.click(screen.getByRole("button", { name: "Create" }));

    const box = screen.getByRole("combobox", { name: "Company" });
    await waitFor(() =>
      expect(box).toHaveAccessibleDescription(
        expect.stringContaining(
          "This record no longer exists, or you cannot see it.",
        ),
      ),
    );
  });
});
