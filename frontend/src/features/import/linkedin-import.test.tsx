/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { LinkedInNetworkPanel } from "./linkedin-import";

// The LinkedIn network panel: the import row says whether anything is imported
// and how much. The profile row offers Add before there is a profile and
// Edit after.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function showing(
  account: Record<string, unknown>,
  extra: Record<string, () => Response> = {},
) {
  installFetchStub({
    ...extra,
    "GET /me": () => jsonResponse(meFixture()),
    "GET /me/linkedin-account": () =>
      jsonResponse({ connected: false, connections: 0, ...account }),
    "GET /me/linkedin-reach": () =>
      jsonResponse({
        accounts: [],
        accounts_total: 0,
        unresolved_connections: 0,
      }),
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <LinkedInNetworkPanel />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("reads Not imported with Add profile on a first visit", async () => {
  showing({ profile_url: "" });

  expect(
    await screen.findByText(en["linkedinImport.notImported"]),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: en["linkedinImport.addProfile"] }),
  ).toBeTruthy();
  expect(screen.getByText(en["linkedinImport.profileNotSet"])).toBeTruthy();
});

it("counts what an import holds and offers Edit once a profile is saved", async () => {
  showing({
    profile_url: "https://www.linkedin.com/in/lars-brandt",
    connected: true,
    connections: 4108,
  });

  expect(await screen.findByText("4,108 imported")).toBeTruthy();
  expect(screen.queryByText(en["linkedinImport.notImported"])).toBeNull();
  expect(
    screen.getByRole("button", { name: en["linkedinImport.editProfile"] }),
  ).toBeTruthy();
});

it("keeps the way to the right file one click away", async () => {
  showing({});

  const summary = await screen.findByText(en["linkedinImport.importLabel"]);
  expect(summary.closest("details")).not.toBeNull();
});

it("imports a dropped export and reports its counts under the row", async () => {
  showing(
    { profile_url: "" },
    {
      "POST /me/linkedin-connections": () =>
        jsonResponse({
          rows: 4120,
          imported: 4108,
          skipped: 12,
          confirmed: 37,
          suggested: 214,
        }),
    },
  );

  await userEvent.upload(
    await screen.findByLabelText(en["linkedinImport.choose"]),
    new File(["First Name,Last Name\n"], "Connections.csv", {
      type: "text/csv",
    }),
  );

  const result = await screen.findByTestId("linkedin-import-result");
  expect(result.textContent).toContain("4,108");
  // Skipped rows are shown, never folded into a success.
  expect(screen.getByText(en["linkedinImport.skipped"])).toBeTruthy();
});
