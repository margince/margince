/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { useRoute } from "../app/router";
import { LocaleProvider } from "../i18n";
import { ImportCard } from "./import";

const RUN_PAGE = "#/settings/import/run";
const REMEMBERED_RUN_KEY = "margince.import.run";

const profile = {
  source_ref: "ws/import/abc",
  object: "lead",
  rows_profiled: 1,
  columns: [{ header: "Email", fill_rate: 1, samples: ["ada@x.test"] }],
  suggested_mapping: { Email: "email" },
  targets: ["full_name", "email"],
};

const parkedRun = {
  id: "019ff-run",
  connector: "csv",
  object: "company",
  status: "complete",
  checkpoint: 1,
  source: "import_api",
  created_at: "2026-08-13T10:00:00Z",
  updated_at: "2026-08-13T10:00:00Z",
};

const parkedReport = {
  run_id: parkedRun.id,
  status: "complete",
  rows_read: 1,
  disposition: { created: 1, updated: 0, unchanged: 0, skipped: 0 },
  issues: [],
  source_key_used: "Email",
};

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });
}

// Answers keyed "METHOD /path"; the parked run's read may be held back.
function stubRoutes(runRead: () => Promise<Response> = async () => json({})) {
  const sent: string[] = [];
  const answers: Record<string, () => Promise<Response> | Response> = {
    "GET /v1/me": () =>
      json(meFixture({ allow: { import_run: ["create", "update", "read"] } })),
    "POST /v1/imports/sources": () => json(profile),
    "GET /v1/imports/019ff-run": runRead,
    "GET /v1/imports/019ff-run/report": () => json(parkedReport),
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(request ? request.url : String(input), "https://t");
      const key = `${request?.method ?? init?.method ?? "GET"} ${url.pathname}`;
      sent.push(key);
      return (answers[key] ?? (() => json({})))();
    }),
  );
  return sent;
}

function RoutedImport() {
  return <ImportCard subpage={useRoute().id2} />;
}

function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RoutedImport />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function profileAFile() {
  await userEvent.upload(
    await screen.findByLabelText("CSV file"),
    new File(["Email\nada@x.test\n"], "estate.csv"),
  );
  await screen.findByRole("row", { name: /Email/ });
}

function pressed(name: string) {
  return screen.getByRole("button", { name, pressed: true });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
  globalThis.location.hash = "";
});

describe("the import page's address", () => {
  it("moves the row type when only the address moves", async () => {
    globalThis.location.hash = `${RUN_PAGE}?object=company`;
    stubRoutes();
    render();
    await screen.findByRole("button", { name: "Companies", pressed: true });

    globalThis.location.hash = `${RUN_PAGE}?object=contact`;

    await waitFor(() => expect(pressed("Contacts")).toBeInTheDocument());
    expect(globalThis.location.hash).toBe(`${RUN_PAGE}?object=contact`);
  });

  it("asks before the address throws a profiled file away, and puts it back when kept", async () => {
    globalThis.location.hash = RUN_PAGE;
    stubRoutes();
    render();
    await profileAFile();

    globalThis.location.hash = `${RUN_PAGE}?object=company`;
    await userEvent.click(
      await screen.findByRole("button", { name: "Cancel" }),
    );

    await waitFor(() =>
      expect(globalThis.location.hash).toBe(`${RUN_PAGE}?object=lead`),
    );
    expect(pressed("Prospects")).toBeInTheDocument();
    expect(screen.getByRole("row", { name: /Email/ })).toBeInTheDocument();
  });

  it("names a recovered run's row type in a bare address", async () => {
    localStorage.setItem(REMEMBERED_RUN_KEY, parkedRun.id);
    globalThis.location.hash = RUN_PAGE;
    stubRoutes(async () => json(parkedRun));
    render();

    await screen.findByText("Import result");
    await waitFor(() =>
      expect(globalThis.location.hash).toBe(`${RUN_PAGE}?object=company`),
    );
  });

  it("keeps the row type chosen before a parked run is read back", async () => {
    localStorage.setItem(REMEMBERED_RUN_KEY, parkedRun.id);
    globalThis.location.hash = RUN_PAGE;
    let answer = (_: Response) => {};
    const sent = stubRoutes(
      () =>
        new Promise((resolve) => {
          answer = resolve;
        }),
    );
    render();
    await userEvent.click(
      await screen.findByRole("button", { name: "Contacts" }),
    );

    await act(async () => answer(json(parkedRun)));
    await waitFor(() =>
      expect(sent).toContain("GET /v1/imports/019ff-run/report"),
    );
    await act(async () => {});

    expect(pressed("Contacts")).toBeInTheDocument();
    expect(screen.queryByText("Import result")).toBeNull();
  });
});

describe("starting the import over", () => {
  it("asks before another row type throws a profiled file away", async () => {
    globalThis.location.hash = RUN_PAGE;
    stubRoutes();
    render();
    await profileAFile();

    await userEvent.click(screen.getByRole("button", { name: "Companies" }));
    expect(screen.getByRole("row", { name: /Email/ })).toBeInTheDocument();
    await userEvent.click(
      await screen.findByRole("button", { name: "Discard changes" }),
    );

    expect(pressed("Companies")).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /Email/ })).toBeNull();
    expect(globalThis.location.hash).toBe(`${RUN_PAGE}?object=company`);
  });

  it("asks before another file replaces a profiled one", async () => {
    globalThis.location.hash = RUN_PAGE;
    const sent = stubRoutes();
    render();
    await profileAFile();
    const uploads = () =>
      sent.filter((key) => key === "POST /v1/imports/sources").length;

    await userEvent.upload(
      screen.getByLabelText("CSV file"),
      new File(["Email\ngrace@x.test\n"], "other.csv"),
    );
    await screen.findByRole("dialog", { name: "Discard unsaved changes?" });
    expect(uploads()).toBe(1);
    await userEvent.click(
      screen.getByRole("button", { name: "Discard changes" }),
    );

    await waitFor(() => expect(uploads()).toBe(2));
  });
});
