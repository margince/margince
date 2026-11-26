/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createQueryClient } from "../app/queryclient";
import { CompanyScreen } from "../features/companies/companies";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { company, company360 } from "./company.fixtures";
import { RecordShell } from "./recordshell.testkit";
import { installFetchStub, jsonResponse, meRoute } from "./story-utils";

// The account scan is the slowest read on the record page. The Tasks tab opens
// it once, and while it runs or after it fails the record stays a record: its
// tabs still lead somewhere.

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

type ScanAnswer = () => Response | Promise<Response>;

// The Tasks tab of an ordinary account, with the scan answered as handed in.
// Every scan request is counted, by method.
function openTasks(scan: ScanAnswer): string[] {
  const asked: string[] = [];
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
    "GET /companies/o-1": () => jsonResponse(company),
    "GET /companies/o-1/360": () => jsonResponse(company360),
    "POST /companies/o-1/scan": () => {
      asked.push("POST");
      return scan();
    },
    "GET /companies/o-1/scan": () => {
      asked.push("GET");
      return scan();
    },
  });
  window.location.hash = "#/companies/o-1/tasks";
  const client = createQueryClient();
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

const neverAnswers: ScanAnswer = () => new Promise<Response>(() => {});

const refuses: ScanAnswer = () =>
  new Response(JSON.stringify({ title: "Unavailable", code: "internal" }), {
    status: 500,
    headers: { "content-type": "application/problem+json" },
  });

async function leaveForTheOverview() {
  fireEvent.click(
    await screen.findByRole("button", { name: en["tab.overview"] }),
  );
  await waitFor(() =>
    expect(window.location.hash).toBe("#/companies/o-1/overview"),
  );
}

describe("CompanyScreen — the account scan", () => {
  it("opens the scan with one request from the Tasks tab", async () => {
    const asked = openTasks(neverAnswers);
    await waitFor(() => expect(asked).toEqual(["POST"]));
    await screen.findByRole("button", { name: en["tab.overview"] });
    expect(asked).toEqual(["POST"]);
  });

  it("keeps the tabs working while the scan is still running", async () => {
    const asked = openTasks(neverAnswers);
    await waitFor(() => expect(asked).toEqual(["POST"]));
    await leaveForTheOverview();
  });

  it("keeps the tabs working after the scan failed", async () => {
    const asked = openTasks(refuses);
    await waitFor(() => expect(asked).toEqual(["POST", "GET"]));
    await leaveForTheOverview();
  });
});
