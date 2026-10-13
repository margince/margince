/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { CompanyScreen } from "./companies";
import { company } from "./company.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

afterEach(cleanup);

const archived = {
  ...company,
  lifecycle: "customer",
  writable: false,
  archived_at: "2026-07-14T09:00:00Z",
};

const notFound = () => jsonResponse({ status: 404, code: "not_found" }, 404);

// Every read the server refuses for an archived account, answered as it does.
// Each is counted, so a spec can say which of them the page asked.
function mountArchived(): string[] {
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
    "/records/company/o-1/lists",
  ]) {
    routes[`GET ${path}`] = refused(path);
  }
  installFetchStub(routes);
  render(
    <StoryProviders>
      <CompanyScreen id="o-1" />
    </StoryProviders>,
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
});
