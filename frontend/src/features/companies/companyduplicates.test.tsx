/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  announceCompanyDuplicates,
  CompanyDuplicatesEntry,
} from "./companyduplicates";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const HERE = "co-new";
const THERE = "co-old";

function pairPage(...pairs: Array<[string, string]>) {
  return {
    data: pairs.map(([left_id, right_id], i) => ({
      id: `pair-${i}`,
      entity_type: "company",
      left_id,
      right_id,
      confidence: 1,
      evidence: [],
      status: "open",
      created_at: "2026-11-01T08:00:00Z",
      can_decide: true,
      can_merge: true,
      keep_id: null,
    })),
    page: { has_more: false },
  };
}

it("names the other company of an open pair on the company page", async () => {
  const asked: string[] = [];
  installFetchStub({
    "GET /me": meRoute({ company: ["read"] }),
    "GET /dedupe/candidates": (_body, url) => {
      asked.push(url.search);
      return jsonResponse(pairPage([THERE, HERE]));
    },
    "GET /records/references": () =>
      jsonResponse({
        data: [
          {
            kind: "company",
            id: THERE,
            name: "Alpha Trading GmbH",
            archived: false,
            logo: null,
          },
        ],
      }),
  });
  render(
    <StoryProviders>
      <CompanyDuplicatesEntry companyId={HERE} />
    </StoryProviders>,
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Possible duplicate (1)" }),
  );
  expect(
    await screen.findByRole("link", { name: "Alpha Trading GmbH" }),
  ).toBeVisible();
  expect(asked[0]).toContain(`record_id=${HERE}`);
  expect(asked[0]).toContain("entity_type=company");
});

it("draws nothing for a company that is in no open pair", async () => {
  installFetchStub({
    "GET /me": meRoute({ company: ["read"] }),
    "GET /dedupe/candidates": () => jsonResponse(pairPage()),
  });
  render(
    <StoryProviders>
      <CompanyDuplicatesEntry companyId={HERE} />
    </StoryProviders>,
  );
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(
    screen.queryByTestId("company-duplicates-entry"),
  ).not.toBeInTheDocument();
});

it("warns the creator with a way to the company that already exists", async () => {
  installFetchStub({
    "GET /dedupe/candidates": () => jsonResponse(pairPage([HERE, THERE])),
  });
  const toast = { show: vi.fn(() => 1), dismiss: vi.fn() };
  await announceCompanyDuplicates(new QueryClient(), toast, (key) => key, HERE);
  expect(toast.show).toHaveBeenCalledTimes(1);
  const [message, options] = toast.show.mock.calls[0] as unknown as [
    string,
    { action: { label: string } },
  ];
  expect(message).toBe("company.duplicates.created");
  expect(options.action.label).toBe("dedupe.viewExisting");
});

it("says nothing when the new company filed no pair", async () => {
  installFetchStub({
    "GET /dedupe/candidates": () => jsonResponse(pairPage()),
  });
  const toast = { show: vi.fn(() => 1), dismiss: vi.fn() };
  await announceCompanyDuplicates(new QueryClient(), toast, (key) => key, HERE);
  expect(toast.show).not.toHaveBeenCalled();
});
