// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it, vi } from "vitest";
import { stable } from "../format/collate";
import { jsonResponse } from "../testing/story-utils";
import { searchByEntity } from "./relationshipcandidates";

afterEach(() => vi.unstubAllGlobals());

const rows = {
  o1: [{ id: "d1", name: "Employer renewal", company_id: "o1" }],
  o2: [{ id: "d2", name: "Second employer expansion", company_id: "o2" }],
  workspace: [
    { id: "d3", name: "Partner renewal", company_id: "o3" },
    { id: "d1", name: "Employer renewal", company_id: "o1" },
  ],
};

function stubDeals() {
  const requests: URL[] = [];
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test.local",
    );
    requests.push(url);
    const company = url.searchParams.get("company_id");
    const status = url.searchParams.get("status");
    const data =
      status === "won"
        ? company === "o2"
          ? []
          : [
              {
                id: "won",
                name: "Won renewal",
                company_id: "o1",
                status: "won",
              },
            ]
        : status === "lost"
          ? company
            ? []
            : [
                {
                  id: "lost",
                  name: "Lost renewal",
                  company_id: "o3",
                  status: "lost",
                },
              ]
          : (company === "o1"
              ? rows.o1
              : company === "o2"
                ? rows.o2
                : rows.workspace
            ).map((row) => ({ ...row, status: "open" }));
    return jsonResponse({ data, page: { has_more: false } });
  });
  return requests;
}

it("lists every current employer's open deals without asking for unrelated deals", async () => {
  const requests = stubDeals();
  const candidates = await searchByEntity("deal", "", ["o1", "o2", "o1"]);
  expect(requests).toHaveLength(2);
  expect(
    requests
      .map((url) => url.searchParams.get("company_id"))
      .sort((a, b) => stable(String(a), String(b))),
  ).toEqual(["o1", "o2"]);
  expect(
    requests.every((url) => url.searchParams.get("status") === "open"),
  ).toBe(true);
  expect(candidates.map((candidate) => candidate.id)).toEqual(["d1", "d2"]);
});

it("searches on the server and places employer deals before other open deals without duplicates", async () => {
  const requests = stubDeals();
  const candidates = await searchByEntity("deal", "renewal", ["o1", "o2"]);
  expect(requests).toHaveLength(9);
  for (const request of requests) {
    expect(request.searchParams.get("q")).toBe("renewal");
    expect(["open", "won", "lost"]).toContain(
      request.searchParams.get("status"),
    );
    expect(request.searchParams.get("limit")).not.toBe("50");
  }
  expect(candidates.map((candidate) => candidate.id)).toEqual([
    "d1",
    "d2",
    "d3",
    "won",
    "lost",
  ]);
  expect(candidates[2]).toMatchObject({ companyId: "o3" });
});

it("searches open and closed deals when no employer is known", async () => {
  const requests = stubDeals();
  await searchByEntity("deal", "older deal");
  expect(requests).toHaveLength(3);
  expect(
    new Set(requests.map((url) => url.searchParams.get("status"))),
  ).toEqual(new Set(["open", "won", "lost"]));
  expect(
    requests.every((url) => url.searchParams.get("q") === "older deal"),
  ).toBe(true);
  expect(requests.every((url) => !url.searchParams.has("company_id"))).toBe(
    true,
  );
});
