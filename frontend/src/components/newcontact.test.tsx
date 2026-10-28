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

// Nordlicht holds nordlicht-logistik.de; no company holds any other domain.
// `createAnswer` answers the contact write.
function stubApi(createAnswer = () => json({ id: "p-new" }, 201)) {
  const posted: unknown[] = [];
  const domainLookups: string[] = [];
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
      if (url.pathname.endsWith("/companies")) {
        const domain = url.searchParams.get("domain");
        if (domain !== null) {
          domainLookups.push(domain);
          return json(
            domain === "nordlicht-logistik.de"
              ? { ...emptyPage, data: [NORDLICHT] }
              : emptyPage,
          );
        }
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
  return { posted, domainLookups };
}

async function openNewContact() {
  const user = userEvent.setup();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
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

  it("leaves the box empty for a domain no company holds", async () => {
    const { posted, domainLookups } = stubApi();
    const user = await openNewContact();

    await typeWorkEmail(user, "dana.buyer@gmail.com");
    await waitFor(() => expect(domainLookups).toContain("gmail.com"));
    expect(screen.getByRole("combobox", { name: "Company" })).toHaveValue("");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).not.toHaveProperty("company_id");
    expect(posted[0]).not.toHaveProperty("company_name");
  });

  it("never replaces a company the reader typed", async () => {
    const { posted, domainLookups } = stubApi();
    const user = await openNewContact();

    await user.type(
      screen.getByRole("combobox", { name: "Company" }),
      "Brandt Automotive GmbH",
    );
    await typeWorkEmail(user, "dana.buyer@nordlicht-logistik.de");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(domainLookups).toEqual([]);
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
