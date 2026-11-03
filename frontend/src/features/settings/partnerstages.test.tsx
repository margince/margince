/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { PartnerStagesCard } from "./partnerstages";

// Settings › Partners: the installation's partner ladder. A stage keeps its
// meaning while partners stand on it, and only an unused custom stage goes.

const ADMIN: GrantSpec = {
  custom_field: ["read", "create", "update", "delete"],
};
const READER: GrantSpec = { custom_field: ["read"] };

function stage(
  key: string,
  label: string,
  meaning: string,
  sort: number,
  extra: Record<string, unknown> = {},
) {
  return {
    id: `stage-${key}`,
    key,
    label,
    meaning,
    sort_order: sort,
    active: true,
    system: true,
    partner_count: 0,
    version: 1,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    ...extra,
  };
}

const STAGES = [
  stage("research", "Research", "candidate", 10, { partner_count: 3 }),
  stage("ready", "Ready", "ready", 80),
  stage("pilot", "Pilot", "candidate", 95, { system: false }),
];

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
    if (request.method === "DELETE") {
      return new Response(null, { status: 204 });
    }
    if (request.method !== "GET") {
      return jsonResponse(STAGES[1], request.method === "POST" ? 201 : 200);
    }
    return jsonResponse({ data: STAGES });
  });
}

function wrote(calls: Call[], method: string, path: string, body?: unknown) {
  return calls.some(
    (call) =>
      call.method === method &&
      new URL(call.url).pathname.endsWith(path) &&
      (body === undefined ||
        JSON.stringify(call.body) === JSON.stringify(body)),
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

it("keeps the meaning of a stage partners stand on and lets an unused one change", async () => {
  vi.stubGlobal("fetch", backend(ADMIN));
  render(<PartnerStagesCard />);
  const inUse = await screen.findByTestId("partner-stage-research");
  await waitFor(() =>
    expect(
      within(inUse).getByRole("combobox", { name: "Meaning of Research" }),
    ).toBeDisabled(),
  );
  expect(
    within(screen.getByTestId("partner-stage-ready")).getByRole("combobox", {
      name: "Meaning of Ready",
    }),
  ).toBeEnabled();
});

it("adds a new stage at the end of the ladder as a candidate", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, calls));
  render(<PartnerStagesCard />);
  await screen.findByTestId("partner-stage-pilot");
  await user.click(screen.getByRole("button", { name: "New stage" }));
  await user.type(await screen.findByRole("textbox"), "Enablement");
  await user.click(screen.getByRole("button", { name: "Add stage" }));
  await waitFor(() =>
    expect(
      wrote(calls, "POST", "/partner-stages", {
        label: "Enablement",
        meaning: "candidate",
        sort_order: 105,
      }),
    ).toBe(true),
  );
});

it("moves a stage down by swapping places with the next one", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, calls));
  render(<PartnerStagesCard />);
  const row = await screen.findByTestId("partner-stage-ready");
  await user.click(within(row).getByRole("button", { name: /Ready/ }));
  await user.click(await screen.findByRole("button", { name: "Move down" }));
  await waitFor(() =>
    expect(
      wrote(calls, "PATCH", "/partner-stages/stage-ready", { sort_order: 95 }),
    ).toBe(true),
  );
  expect(
    wrote(calls, "PATCH", "/partner-stages/stage-pilot", { sort_order: 80 }),
  ).toBe(true);
});

it("offers removal only for a custom stage no partner stands on", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, calls));
  render(<PartnerStagesCard />);
  const builtIn = await screen.findByTestId("partner-stage-ready");
  await user.click(within(builtIn).getByRole("button", { name: /Ready/ }));
  expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
  await user.keyboard("{Escape}");

  const custom = screen.getByTestId("partner-stage-pilot");
  await user.click(within(custom).getByRole("button", { name: /Pilot/ }));
  await user.click(await screen.findByRole("button", { name: "Remove" }));
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("button", { name: "Remove" }));
  await waitFor(() =>
    expect(wrote(calls, "DELETE", "/partner-stages/stage-pilot")).toBe(true),
  );
});

it("shows the ladder to a reader without a control that writes", async () => {
  vi.stubGlobal("fetch", backend(READER));
  render(<PartnerStagesCard />);
  const row = await screen.findByTestId("partner-stage-ready");
  expect(within(row).getByRole("switch")).toBeDisabled();
  expect(screen.queryByRole("button", { name: "New stage" })).toBeNull();
});

it("leaves the meaning to the server's answer when the count is withheld", async () => {
  const withheld = STAGES.map(({ partner_count: _count, ...rest }) => rest);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) =>
      new URL(request.url).pathname.endsWith("/me")
        ? jsonResponse(meFixture({ allow: ADMIN }))
        : jsonResponse({ data: withheld }),
    ),
  );
  render(<PartnerStagesCard />);
  const row = await screen.findByTestId("partner-stage-research");
  await waitFor(() =>
    expect(
      within(row).getByRole("combobox", { name: "Meaning of Research" }),
    ).toBeEnabled(),
  );
});
