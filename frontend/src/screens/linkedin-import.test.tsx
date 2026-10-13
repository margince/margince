/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { LinkedInNetworkPanel } from "./linkedin-import";
import { installFetchStub, jsonResponse } from "./story-utils";

// The LinkedIn network panel: the import row says whether anything is imported
// and how much, and the profile row offers Add before there is a profile and
// Edit after.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function showing(account: Record<string, unknown>) {
  installFetchStub({
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
  expect(screen.getByText(en["linkedinReach.empty"])).toBeTruthy();
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
