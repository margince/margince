// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { jsonResponse } from "../../testing/company.fixtures";
import { project } from "../../testing/projects.fixtures";
import { RecordShell } from "../../testing/recordshell.testkit";
import { ProjectScreen } from "./project360";
import { project360Key } from "./project360.key";
import type { ArchivedHistory } from "./projectarchived";
import { projectsBackend } from "./projects.testing";

// The project page for a project the 360 refuses. An archived one opens from
// its row, read-only. An address that names no project says so and leads back
// to the list instead of offering a retry that cannot help.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

let client: QueryClient;

function render(ui: ReactNode) {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const notFound = () =>
  jsonResponse({ title: "Not found", status: 404, detail: "not found" }, 404);

const refused360 = async (url: string) =>
  url.endsWith("/360") ? notFound() : null;

describe("ProjectScreen on a project the 360 refuses", () => {
  it("opens an archived project read-only from its row", async () => {
    projectsBackend({
      projects: [project({ archived_at: "2026-11-01T09:00:00Z" })],
      respond: refused360,
    });
    // The shell passes the records feature's panel in; it gets the row's version.
    const History: ArchivedHistory = ({ kind, id, version }) => (
      <p>{`${kind} ${id} history at version ${version}`}</p>
    );
    render(<ProjectScreen id="pr-1" archivedHistory={History} />);
    expect(
      await screen.findByRole("heading", { name: "CRM rollout" }),
    ).toBeTruthy();
    expect(screen.getByText(en["record.archived"])).toBeTruthy();
    expect(screen.getByText(en["project.archivedReadOnly"])).toBeTruthy();
    expect(screen.getByText("project pr-1 history at version 3")).toBeTruthy();
    expect(screen.queryByText(en["common.error"])).toBeNull();
  });

  it("says a project that does not exist is not found, with a way back", async () => {
    projectsBackend({
      respond: async (url) =>
        url.includes("/projects/pr-missing") ? notFound() : null,
    });
    render(<ProjectScreen id="pr-missing" />);
    expect(
      await screen.findByRole("heading", { name: en["project.page.notFound"] }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: en["project.page.backToProjects"] }),
    ).toBeTruthy();
    expect(screen.queryByText(en["common.error"])).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["common.retry"] }),
    ).toBeNull();
  });

  it("opens the project once its 360 answers, though the row was refused before", async () => {
    let refused = true;
    projectsBackend({
      respond: async (url) =>
        refused && url.includes("/projects/pr-1") ? notFound() : null,
    });
    render(<ProjectScreen id="pr-1" />);
    expect(
      await screen.findByRole("heading", { name: en["project.page.notFound"] }),
    ).toBeTruthy();

    refused = false;
    await client.invalidateQueries({ queryKey: project360Key("pr-1") });
    expect(
      await screen.findByRole("heading", { name: "CRM rollout" }),
    ).toBeTruthy();
  });

  it("reads an address that names no project as not found", async () => {
    projectsBackend({
      respond: async (url) =>
        url.includes("/projects/not-a-uuid")
          ? jsonResponse(
              { title: "Invalid", status: 422, detail: "malformed" },
              422,
            )
          : null,
    });
    render(<ProjectScreen id="not-a-uuid" />);
    expect(
      await screen.findByRole("heading", { name: en["project.page.notFound"] }),
    ).toBeTruthy();
  });
});

describe("ProjectScreen on a live project", () => {
  it("reads its 360 and never its row", async () => {
    const { urls } = projectsBackend();
    render(<ProjectScreen id="pr-1" />);
    expect(
      await screen.findByRole("heading", { name: "CRM rollout" }),
    ).toBeTruthy();
    const paths = urls.map((url) => new URL(url).pathname);
    expect(paths).toContain("/v1/projects/pr-1/360");
    expect(paths).not.toContain("/v1/projects/pr-1");
  });
});
