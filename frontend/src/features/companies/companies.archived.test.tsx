/** @vitest-environment happy-dom */
import { type QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createQueryClient } from "../../app/queryclient";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { company, company360 } from "../../testing/company.fixtures";
import { RecordShell } from "../../testing/recordshell.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
} from "../../testing/story-utils";
import { CompanyScreen } from "./companies";

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

const archived = {
  ...company,
  lifecycle: "customer",
  writable: false,
  archived_at: "2026-07-14T09:00:00Z",
};

const notFound = () => jsonResponse({ status: 404, code: "not_found" }, 404);

function activity(id: string, subject: string) {
  return {
    id,
    kind: "call",
    subject,
    occurred_at: "2026-07-10T09:00:00Z",
    is_done: true,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-07-10T09:00:00Z",
    updated_at: "2026-07-10T09:00:00Z",
  };
}

// Every read the server refuses for an archived account, answered as it does.
// Each is counted, so a spec can say which of them the page asked.
function mountArchived(
  options: Readonly<{
    tab?: string;
    routes?: RouteMap;
    client?: QueryClient;
  }> = {},
): string[] {
  const asked: string[] = [];
  const refused = (path: string) => () => {
    asked.push(path);
    return notFound();
  };
  const routes: RouteMap = {
    "GET /me": meRoute({ company: ["read", "update"] }),
    "GET /companies/o-1": () => jsonResponse(archived),
    "GET /companies/o-1/capture-triage": () =>
      jsonResponse({ company_id: "o-1", domains: [] }),
  };
  for (const path of [
    "/companies/o-1/360",
    "/companies/o-1/finance-summary",
    "/companies/o-1/contracts",
    "/records/company/o-1/lists",
  ]) {
    routes[`GET ${path}`] = refused(path);
  }
  installFetchStub({ ...routes, ...options.routes });
  window.location.hash = `#/companies/o-1/${options.tab ?? "overview"}`;
  const client = options.client ?? createQueryClient();
  client.setDefaultOptions({
    queries: { ...client.getDefaultOptions().queries, retry: false },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>
          <CompanyScreen id="o-1" />
        </RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return asked;
}

describe("CompanyScreen — an archived account", () => {
  it("reads as archived, and no section reads as one that failed to load", async () => {
    const asked = mountArchived();

    expect(await screen.findByText(en["record.archived"])).toBeTruthy();
    await waitFor(() => expect(asked).toContain("/companies/o-1/360"));
    expect(await screen.findByText(en["record.archivedReadOnly"])).toBeTruthy();
    for (const failure of [
      en["co.partial"],
      en["co.section.unavailable"],
      en["state.unavailable"],
      en["state.failed"],
      en["today.failed"],
    ]) {
      expect(screen.queryByText(failure)).toBeNull();
    }
    expect(asked).toEqual(["/companies/o-1/360"]);
  });

  it("offers no tab whose content the server refuses", async () => {
    mountArchived();

    expect(
      await screen.findByRole("button", { name: en["tab.overview"] }),
    ).toBeTruthy();
    for (const tab of [
      en["tab.contacts"],
      en["tab.dealsProjects"],
      en["tab.tasks"],
      en["tab.finance"],
    ]) {
      expect(screen.queryByRole("button", { name: tab })).toBeNull();
    }
  });

  for (const refusedTab of ["contacts", "deals", "tasks", "finance"]) {
    it(`opens a link to its ${refusedTab} tab on the Overview`, async () => {
      mountArchived({ tab: refusedTab });

      const overview = await screen.findByRole("button", {
        name: en["tab.overview"],
      });
      expect(overview.getAttribute("aria-pressed")).toBe("true");
      await waitFor(() =>
        expect(window.location.hash).toBe("#/companies/o-1/overview"),
      );
    });
  }

  it("draws its timeline from the activity list, not from a 360 it never shows", async () => {
    mountArchived({
      tab: "timeline",
      routes: {
        "GET /companies/o-1/360": () =>
          jsonResponse({
            ...company360,
            activities: {
              data: [activity("a-360", "From the composite")],
              page: { has_more: false },
            },
          }),
        "GET /activities": () =>
          jsonResponse({
            data: [activity("a-list", "From the list")],
            page: { has_more: false },
          }),
      },
    });

    expect(await screen.findByText("From the list")).toBeTruthy();
    expect(screen.queryByText("From the composite")).toBeNull();
  });

  it("waits on nothing when its 360 is never asked", async () => {
    const client = createQueryClient();
    client.setQueryData(["company", "o-1"], archived);
    mountArchived({
      tab: "timeline",
      client,
      routes: {
        "GET /activities": () =>
          jsonResponse({
            data: [activity("a-list", "From the list")],
            page: { has_more: false },
          }),
      },
    });

    expect(await screen.findByText("From the list")).toBeTruthy();
    expect(screen.queryByText(en["record.chronologyLoading"])).toBeNull();
  });

  it("asks for no 360 once the row is known archived", async () => {
    const client = createQueryClient();
    client.setQueryData(["company", "o-1"], archived);
    const asked = mountArchived({ client });

    expect(await screen.findByText(en["record.archivedReadOnly"])).toBeTruthy();
    expect(asked).not.toContain("/companies/o-1/360");
  });

  it("reads no contracts on its documents tab", async () => {
    const asked = mountArchived({ tab: "documents" });

    expect(await screen.findByText(en["record.archived"])).toBeTruthy();
    await waitFor(() => expect(asked).toContain("/companies/o-1/360"));
    expect(screen.queryByText(en["contracts.title"])).toBeNull();
    expect(asked).not.toContain("/companies/o-1/contracts");
  });
});
