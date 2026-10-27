// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { CompanyReachCard } from "./company-reach";

// Who on the team can get you in at this account. The card names the colleague
// to ask and says how many connections they have here, and in which functions.
// Never who they are.

type Reach = components["schemas"]["CompanyLinkedInReachResponse"];

const COMPANY = "018f3a1b-0000-7000-8000-0000000000c1";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderCard(body: Reach): void {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    [`GET /companies/${COMPANY}/linkedin-reach`]: () => jsonResponse(body),
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyReachCard companyId={COMPANY} companyName="Acme GmbH" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("names each colleague, where their connections work and their count, in the order the server sent", async () => {
  renderCard({
    colleagues: [
      {
        user_id: "018f3a1b-0000-7000-8000-0000000000u1",
        display_name: "Dana Okonjo",
        connections: 3,
        functions: [
          { function: "it", connections: 2 },
          { function: "finance", connections: 2 },
        ],
      },
      {
        user_id: "018f3a1b-0000-7000-8000-0000000000u2",
        display_name: "Minh Tran",
        connections: 1,
        functions: [],
      },
    ],
  });

  const table = await screen.findByRole("table");
  const [, ...rows] = within(table).getAllByRole("row");
  expect(
    rows.map((row) =>
      within(row)
        .getAllByRole("cell")
        .map(
          (cell) =>
            (cell.querySelector(".datatable-unfolded") ?? cell).textContent,
        ),
    ),
  ).toEqual([
    ["Dana Okonjo", "IT · 2, Finance · 2", "3", "Ask for an introduction"],
    ["Minh Tran", "", "1", "Ask for an introduction"],
  ]);
});

it("says nobody reaches this account rather than drawing an empty table", async () => {
  renderCard({ colleagues: [] });

  expect(
    await screen.findByText(/No colleague reach is shown for this company/),
  ).toBeInTheDocument();
  expect(screen.queryByRole("table")).toBeNull();
});

// A refusal is not an empty answer. Drawn as "nobody reaches this account" it
// would read as a cold door rather than a list this reader was not given.
it("says the list could not be read when the read is refused", async () => {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    [`GET /companies/${COMPANY}/linkedin-reach`]: () =>
      jsonResponse({ title: "Forbidden", detail: "no" }, 403),
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyReachCard companyId={COMPANY} companyName="Acme GmbH" />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  expect(
    await screen.findByText(/reach list could not be read/),
  ).toBeInTheDocument();
  expect(screen.queryByRole("table")).toBeNull();
});

// The reader's own reach is a way in they hold already. It stays, reads "You"
// rather than their own name, and leads the list.
it("names the reader's own row You, first", async () => {
  const me = meFixture();
  renderCard({
    colleagues: [
      {
        user_id: "018f3a1b-0000-7000-8000-0000000000u2",
        display_name: "Minh Tran",
        connections: 1,
        functions: [],
      },
      {
        user_id: me.user.id,
        display_name: me.user.display_name,
        connections: 8,
        functions: [],
      },
    ],
  });

  const table = await screen.findByRole("table");
  const [, ...rows] = within(table).getAllByRole("row");
  const firstCell = await within(rows[0]).findByText("You");
  expect(firstCell).toBeInTheDocument();
  expect(within(table).queryByText(me.user.display_name)).toBeNull();
  expect(within(rows[1]).getByText("Minh Tran")).toBeInTheDocument();
});

// The ask names the account and, when the reader picks one, the function, and
// never a contact: the colleague's connections are not on record here.
it("asks a colleague for an introduction at the account in the function the reader picks", async () => {
  const user = userEvent.setup();
  let sent: unknown;
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    [`GET /companies/${COMPANY}/linkedin-reach`]: () =>
      jsonResponse({
        colleagues: [
          {
            user_id: "018f3a1b-0000-7000-8000-0000000000u1",
            display_name: "Dana Okonjo",
            connections: 3,
            functions: [{ function: "it", connections: 2 }],
          },
        ],
      } satisfies Reach),
    [`POST /companies/${COMPANY}/intro-request-draft`]: (body) => {
      sent = body;
      return jsonResponse({
        subject: "Could you introduce me to someone at Acme GmbH?",
        body: "Hi Dana,\n\nWould you be up for introducing me to someone there who works in IT?",
        generated_by: "deterministic",
      });
    },
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyReachCard companyId={COMPANY} companyName="Acme GmbH" />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  await user.click(
    await screen.findByRole("button", {
      name: "Ask Dana Okonjo for an introduction",
    }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(
    within(dialog).getByText(
      "Asking Dana Okonjo to introduce you to someone at Acme GmbH.",
    ),
  ).toBeInTheDocument();
  await user.click(within(dialog).getByRole("combobox"));
  await user.click(await screen.findByRole("option", { name: "IT · 2" }));
  await user.click(
    within(dialog).getByRole("button", { name: "Draft message" }),
  );

  expect(
    await within(dialog).findByDisplayValue(
      "Could you introduce me to someone at Acme GmbH?",
    ),
  ).toBeInTheDocument();
  expect(sent).toEqual({
    via_user_id: "018f3a1b-0000-7000-8000-0000000000u1",
    function: "it",
  });
});

it("offers the reader no ask of themselves", async () => {
  const me = meFixture();
  renderCard({
    colleagues: [
      {
        user_id: me.user.id,
        display_name: me.user.display_name,
        connections: 2,
        functions: [],
      },
    ],
  });

  await screen.findByText("You");
  expect(
    screen.queryByRole("button", { name: /for an introduction/ }),
  ).toBeNull();
});
