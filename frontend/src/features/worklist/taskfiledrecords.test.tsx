/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { companyNamesAnswer } from "../../testing/companynames.testkit";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { openTask, taskMe } from "./taskdetail.testkit";

afterEach(cleanup);

function stubFiledTask(links: { entity_type: string; entity_id: string }[]) {
  installFetchStub({
    "GET /me": taskMe("update"),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        writable: true,
        subject: "Send the renewal paperwork",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        links,
      }),
    "GET /deals/deal-1": () =>
      jsonResponse({ id: "deal-1", name: "Brandt renewal 2027" }),
    "GET /companies": () =>
      jsonResponse({
        data: [{ id: "co-1", display_name: "Brandt Automotive" }],
        next_cursor: null,
      }),
  });
  const recordFetch = globalThis.fetch;
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(request?.url ?? String(input), "https://test.local");
    const method = request?.method ?? init?.method ?? "GET";
    if (method === "GET") {
      const company = companyNamesAnswer(url.href, {
        "co-1": "Brandt Automotive",
      });
      if (company) return Promise.resolve(company);
      if (
        url.pathname === "/v1/records/references" &&
        url.searchParams.get("kind") === "deal"
      ) {
        return Promise.resolve(
          jsonResponse({
            data: url.searchParams
              .getAll("id")
              .flatMap((id) =>
                id === "deal-1"
                  ? [{ id, name: "Brandt renewal 2027", archived: false }]
                  : [],
              ),
          }),
        );
      }
    }
    return recordFetch(input, init);
  });
}

test("a task filed under a deal and its company names the deal first", async () => {
  // The company first on the wire, so the order on screen is the panel's own.
  stubFiledTask([
    { entity_type: "company", entity_id: "co-1" },
    { entity_type: "deal", entity_id: "deal-1" },
  ]);
  openTask();

  const deal = await screen.findByRole("link", { name: "Brandt renewal 2027" });
  await screen.findByRole("link", { name: "Brandt Automotive" });
  const chips = screen.getAllByRole("link");
  expect(chips.map((chip) => chip.className)).toEqual([
    "chip chip-dense chip-wrap chip-link",
    "chip chip-dense chip-lead chip-wrap chip-link",
  ]);
  // The company chip leads with its avatar, the deal chip with its kind's icon.
  expect(chips[1]?.querySelector(".chip-mark")?.textContent).toBe("BA");
  expect(chips.map((chip) => chip.lastElementChild?.textContent)).toEqual([
    "Brandt renewal 2027",
    "Brandt Automotive",
  ]);
  // Opened in place: following the record is leaving the panel for it.
  expect(deal.getAttribute("href")).toBe("/#/deals/deal-1");
  expect(deal.getAttribute("target")).toBeNull();
});

test("a task filed under nothing names no record", async () => {
  stubFiledTask([]);
  openTask();

  await screen.findByText(/^Logged/);
  expect(screen.queryByRole("link")).toBeNull();
});
