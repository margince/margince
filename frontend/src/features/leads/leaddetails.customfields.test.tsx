/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadDetails } from "./leaddetails";
import { useLeadPatch } from "./leads.patch";
import { lead } from "./leads.testkit";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

const budgetNote = {
  id: "cf-1",
  object: "lead",
  label: "Budget note",
  slug: "budget_note",
  column_name: "cf_budget_note",
  type: "text",
  status: "active",
  show_in_create: true,
};

const withValue = { ...lead, cf_budget_note: "around 20k", version: 3 };

function Details({ record }: Readonly<{ record: typeof withValue }>) {
  const writer = useLeadPatch(record, record.id, () => undefined);
  return (
    <LeadDetails lead={record} writer={writer} terminalReasonId="refused" />
  );
}

function show(record: typeof withValue) {
  const patches: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute(
      { lead: ["read", "update"], list: ["read", "update"] },
      { seat: "full" },
    ),
    "GET /custom-fields": () =>
      jsonResponse({ page: { has_more: false }, data: [budgetNote] }),
    [`GET /records/lead/${record.id}/lists`]: () => jsonResponse({ data: [] }),
    [`PATCH /leads/${record.id}`]: (body) => {
      patches.push(body);
      return jsonResponse({ ...record, ...(body as object), version: 4 });
    },
  });
  render(
    <StoryProviders>
      <Details record={record} />
    </StoryProviders>,
  );
  return patches;
}

describe("a lead's custom fields", () => {
  it("shows the value typed in New lead on the lead page", async () => {
    show(withValue);
    expect(
      await screen.findByRole("button", {
        name: "Change Budget note around 20k",
      }),
    ).toBeTruthy();
  });

  it("saves a changed value as that field alone", async () => {
    const user = userEvent.setup();
    const patches = show(withValue);
    await user.click(
      await screen.findByRole("button", {
        name: "Change Budget note around 20k",
      }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Budget note" });
    const box = within(dialog).getByRole("textbox", { name: "Budget note" });
    await user.clear(box);
    await user.type(box, "around 30k{Enter}");
    await waitFor(() =>
      expect(patches).toEqual([{ cf_budget_note: "around 30k" }]),
    );
  });
});
