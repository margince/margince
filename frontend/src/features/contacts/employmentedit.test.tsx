/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { EmploymentEdit } from "./employmentedit";

type Employment = components["schemas"]["Contact360Employment"];

const role: Employment = {
  relationship_id: "role",
  company_id: "company",
  is_current_primary: false,
  role: "Advisor",
  employment_status: "former",
  started_at: "2020-02-01",
  started_precision: "month",
  version: 3,
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function sentPatches() {
  const patches: { path: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      const path = new URL(request.url).pathname;
      if (request.method === "PATCH") {
        patches.push({ path, body: await request.json() });
        return Response.json({ id: "role", version: 4 });
      }
      return Response.json({ code: "not_found" }, { status: 404 });
    }),
  );
  return patches;
}

function open(employment: Employment, onSaved = async () => {}) {
  const onClose = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <EmploymentEdit
          employment={employment}
          contactId="contact"
          onClose={onClose}
          onSaved={onSaved}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { onClose };
}

it("sends a typed month as the first of that month, with month precision", async () => {
  const patches = sentPatches();
  const onSaved = vi.fn(async () => {});
  const { onClose } = open(role, onSaved);
  const user = userEvent.setup();

  await user.clear(screen.getByLabelText("Role"));
  await user.type(screen.getByLabelText("Role"), "Chair");
  await user.clear(screen.getByLabelText("Start date"));
  await user.type(screen.getByLabelText("Start date"), "2021-03");
  await user.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(onSaved).toHaveBeenCalled();
  expect(patches).toHaveLength(1);
  expect(patches[0]?.path).toMatch(/\/relationships\/role$/);
  expect(patches[0]?.body).toMatchObject({
    role: "Chair",
    started_at: "2021-03-01",
    started_precision: "month",
  });
});

it("clears a start date the reader emptied, rather than leaving it", async () => {
  const patches = sentPatches();
  const { onClose } = open(role);
  const user = userEvent.setup();

  await user.clear(screen.getByLabelText("Start date"));
  await user.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(patches[0]?.body).toMatchObject({ clear_started_at: true });
  expect(patches[0]?.body).not.toHaveProperty("started_at");
});

it("refuses to save a date the wire cannot carry", async () => {
  sentPatches();
  open(role);
  const user = userEvent.setup();

  await user.type(screen.getByLabelText("End date"), "soon");

  expect(screen.getByRole("button", { name: "Save" })).toHaveProperty(
    "disabled",
    true,
  );
});

it("offers the current-employer box only for a role that is still held", () => {
  sentPatches();
  open(role);

  expect(
    screen.getByRole("checkbox", { name: "This is their current employer" }),
  ).toHaveProperty("disabled", true);
});
