/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { en } from "../i18n/en";
import { AcquisitionSourcesCard } from "./acquisitionsources";
import { jsonResponse, render } from "./settings.testkit";

// Settings › Acquisition sources: every role reads the list; the custom_field
// write verbs decide who may change it, and the card disables rather than hides.

const ADMIN: GrantSpec = { custom_field: ["read", "create", "update"] };
const READER: GrantSpec = { custom_field: ["read"] };

const CONFLICT = {
  type: "about:blank",
  title: "Conflict",
  status: 409,
  code: "conflict",
  detail: "conflict",
};

function source(key: string, label: string, extra: object = {}) {
  return {
    id: `acq-${key}`,
    key,
    label,
    sort_order: 10,
    active: true,
    system: true,
    deal_count: 0,
    version: 1,
    created_at: "2026-08-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    ...extra,
  };
}

type Call = { method: string; url: string; body: unknown };

function backend(
  allow: GrantSpec,
  calls: Call[] = [],
  options: Readonly<{ counted?: boolean; refuseWrite?: boolean }> = {},
) {
  const { counted = true, refuseWrite = false } = options;
  return vi.fn(async (request: Request) => {
    const raw = request.method === "GET" ? "" : await request.text();
    calls.push({
      method: request.method,
      url: request.url,
      body: raw ? JSON.parse(raw) : undefined,
    });
    if (request.url.endsWith("/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (request.method !== "GET") {
      return refuseWrite
        ? jsonResponse(CONFLICT, 409)
        : jsonResponse(source("partner", "Partner"));
    }
    const rows = [
      source("existing_customer", "Existing customer", { deal_count: 12 }),
      source("partner", "Partner", { deal_count: 1 }),
    ];
    return jsonResponse({
      data: counted
        ? rows
        : rows.map((row) => ({ ...row, deal_count: undefined })),
    });
  });
}

function wrote(calls: Call[], method: string, path: string, body: unknown) {
  return calls.some(
    (call) =>
      call.method === method &&
      call.url.endsWith(path) &&
      JSON.stringify(call.body) === JSON.stringify(body),
  );
}

const actionsFor = (label: string) =>
  en["table.rowActions"].replace("{name}", label);

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

it("names each source over its key and counts its deals, with no text box and no Built-in badge", async () => {
  vi.stubGlobal("fetch", backend(ADMIN));
  render(<AcquisitionSourcesCard />);
  const row = await screen.findByTestId("acq-source-existing_customer");
  expect(within(row).getByText("Existing customer")).toBeInTheDocument();
  expect(within(row).getByText("existing_customer")).toBeInTheDocument();
  expect(within(row).getByText("12")).toBeInTheDocument();
  expect(within(row).getByText("12 deals")).toBeInTheDocument();
  expect(within(row).queryByRole("textbox")).toBeNull();
  // No remove verb, so built-in changes nothing a seat can do.
  expect(screen.queryByText("Built-in")).toBeNull();
  const cells = [...row.querySelectorAll("td")];
  expect(cells[0]?.getAttribute("data-fold")).toBe("title");
  expect(cells.at(-1)?.getAttribute("data-fold")).toBe("end");
});

it("says a withheld deal count in words and never as a zero", async () => {
  vi.stubGlobal("fetch", backend(ADMIN, [], { counted: false }));
  render(<AcquisitionSourcesCard />);
  const row = await screen.findByTestId("acq-source-partner");
  expect(
    within(row).getByText(en["acqSources.dealsWithheld"]),
  ).toBeInTheDocument();
  expect(within(row).queryByText("0")).toBeNull();
  expect(within(row).queryByText(/0 deals/)).toBeNull();
});

it("renames through the row menu's dialog, starting from the current label", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, calls));
  render(<AcquisitionSourcesCard />);
  await user.click(
    await screen.findByRole("button", { name: actionsFor("Partner") }),
  );
  expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
  await user.click(screen.getByRole("button", { name: "Rename" }));
  const dialog = within(await screen.findByRole("dialog"));
  const field = dialog.getByLabelText(en["acqSources.addLabel"]);
  expect(field).toHaveValue("Partner");
  await user.clear(field);
  await user.type(field, "Channel partner{Enter}");
  await waitFor(() =>
    expect(
      wrote(calls, "PATCH", "/acquisition-sources/acq-partner", {
        label: "Channel partner",
      }),
    ).toBe(true),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

it("adds a source through the header verb's dialog", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, calls));
  render(<AcquisitionSourcesCard />);
  await user.click(
    await screen.findByRole("button", { name: en["acqSources.addOpen"] }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  await user.type(
    dialog.getByLabelText(en["acqSources.addLabel"]),
    " Roadshow {Enter}",
  );
  await waitFor(() =>
    expect(
      wrote(calls, "POST", "/acquisition-sources", { label: "Roadshow" }),
    ).toBe(true),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

it("says a duplicate name on the field and keeps the dialog open", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("fetch", backend(ADMIN, [], { refuseWrite: true }));
  render(<AcquisitionSourcesCard />);
  await user.click(
    await screen.findByRole("button", { name: en["acqSources.addOpen"] }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  const field = dialog.getByLabelText(en["acqSources.addLabel"]);
  await user.type(field, "Partner{Enter}");
  expect(await dialog.findByText(en["acqSources.duplicate"])).toBeTruthy();
  expect(field.getAttribute("aria-invalid")).toBe("true");
  // A changed name is a new attempt, so the refusal leaves the field.
  await user.type(field, "s");
  expect(dialog.queryByText(en["acqSources.duplicate"])).toBeNull();
});

it("retires a source through its switch, one PATCH", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, calls));
  render(<AcquisitionSourcesCard />);
  await user.click(
    await screen.findByRole("switch", {
      name: "Partner can be chosen on a deal",
    }),
  );
  await waitFor(() =>
    expect(
      wrote(calls, "PATCH", "/acquisition-sources/acq-partner", {
        active: false,
      }),
    ).toBe(true),
  );
  expect(calls.filter((call) => call.method === "PATCH")).toHaveLength(1);
});

it("leaves every control inert for a reader and says why", async () => {
  vi.stubGlobal("fetch", backend(READER));
  render(<AcquisitionSourcesCard />);
  expect(
    await screen.findByRole("switch", {
      name: "Partner can be chosen on a deal",
    }),
  ).toBeDisabled();
  expect(screen.getByText(en["leadSources.readOnlyTitle"])).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: en["acqSources.addOpen"] }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: actionsFor("Partner") }),
  ).toBeNull();
});
