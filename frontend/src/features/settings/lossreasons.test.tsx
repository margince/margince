/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { LegacyLossReasonsCard, LossReasonsCard } from "./lossreasons";

// Settings › Loss reasons: the list a closer picks from, and the review that
// maps the notes of deals lost before it to a reason.

const ADMIN: GrantSpec = {
  custom_field: ["read", "create", "update"],
  data_task: ["read", "update"],
};
const DECIDER: GrantSpec = { custom_field: ["read", "update"] };
const READER: GrantSpec = { custom_field: ["read"] };

function reason(key: string, label: string, active = true) {
  return {
    id: `reason-${key}`,
    key,
    label,
    sort_order: 10,
    active,
    system: true,
    deal_count: 3,
    version: 1,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
  };
}

const HASH = "a".repeat(64);

type Call = { method: string; url: string; body: unknown };

function backend(allow: GrantSpec, calls: Call[] = []) {
  return vi.fn(async (request: Request) => {
    const raw = request.method === "GET" ? "" : await request.text();
    calls.push({
      method: request.method,
      url: request.url,
      body: raw ? JSON.parse(raw) : undefined,
    });
    const url = new URL(request.url);
    if (url.pathname.endsWith("/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (url.pathname.endsWith("/loss-reasons/legacy/apply")) {
      return jsonResponse({ mappings: 1, deals: 7 }, 202);
    }
    if (url.pathname.endsWith("/loss-reasons/legacy")) {
      return jsonResponse({
        data: [
          {
            text_hash: HASH,
            text: "Their quote was cheaper",
            deal_count: 7,
            mapping: {
              id: "m-1",
              text_hash: HASH,
              reason_key: "price",
              decided_by: "human:x",
              decided_at: "2026-10-02T08:00:00Z",
              version: 1,
            },
          },
          {
            text_hash: "b".repeat(64),
            text: "The champion left",
            deal_count: 2,
          },
        ],
        distinct_total: 9,
      });
    }
    if (url.pathname.includes("/loss-reasons/legacy/")) {
      return jsonResponse({
        id: "m-2",
        text_hash: "b".repeat(64),
        reason_key: "no_decision",
        decided_by: "human:x",
        decided_at: "2026-10-02T08:00:00Z",
        version: 1,
      });
    }
    if (request.method === "PATCH") {
      return jsonResponse(reason("timing", "Bad timing", false));
    }
    return jsonResponse({
      data: [
        reason("price", "Price"),
        reason("no_decision", "No decision"),
        reason("timing", "Bad timing", false),
      ],
    });
  });
}

function wrote(calls: Call[], method: string, path: string, body: unknown) {
  return calls.some(
    (call) =>
      call.method === method &&
      new URL(call.url).pathname.endsWith(path) &&
      JSON.stringify(call.body) === JSON.stringify(body),
  );
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

it("retires a reason through its switch rather than deleting it", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, calls));
  render(<LossReasonsCard />);
  const row = await screen.findByTestId("loss-reason-price");
  await user.click(within(row).getByRole("switch"));
  await waitFor(() =>
    expect(
      wrote(calls, "PATCH", "/loss-reasons/reason-price", { active: false }),
    ).toBe(true),
  );
});

it("maps a legacy note to an active reason and applies the decisions", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, calls));
  render(<LegacyLossReasonsCard />);
  expect(
    await screen.findByText("Showing the 2 most common of 9 notes."),
  ).toBeInTheDocument();
  const pick = await screen.findByRole("combobox", {
    name: "Reason for “The champion left”",
  });
  await user.click(pick);
  await screen.findByRole("option", { name: "No decision" });
  expect(screen.queryByRole("option", { name: "Bad timing" })).toBeNull();
  await user.keyboard("{Escape}");
  await pickOption(user, pick, "No decision");
  await waitFor(() =>
    expect(
      wrote(calls, "PUT", `/loss-reasons/legacy/${"b".repeat(64)}`, {
        reason_key: "no_decision",
      }),
    ).toBe(true),
  );

  await user.click(
    screen.getByRole("button", { name: en["lossReasons.apply"] }),
  );
  expect(
    await screen.findByText("7 deals will be updated in the background."),
  ).toBeInTheDocument();
});

it("offers the decisions but not the apply to a seat that may not run a correction", async () => {
  vi.stubGlobal("fetch", backend(DECIDER));
  render(<LegacyLossReasonsCard />);
  await screen.findByText("The champion left");
  expect(
    screen.queryByRole("button", { name: en["lossReasons.apply"] }),
  ).toBeNull();
});

it("shows no legacy review to a seat that may only read the list", async () => {
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(READER, calls));
  const view = render(
    <>
      <LossReasonsCard />
      <LegacyLossReasonsCard />
    </>,
  );
  await screen.findByTestId("loss-reason-price");
  expect(view.container.textContent).not.toContain(
    en["lossReasons.legacyTitle"],
  );
  expect(calls.some((call) => call.url.includes("/loss-reasons/legacy"))).toBe(
    false,
  );
});
