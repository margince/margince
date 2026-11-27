// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { ConsentPurposesCard } from "./privacy.purposes";

const purpose = {
  id: "p1",
  key: "newsletter",
  label: "Old label",
  class: "marketing",
  created_at: "2026-01-01T00:00:00Z",
};
beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});
function setup(options: { writable?: boolean; refused?: boolean } = {}) {
  let rows = [purpose];
  const writes: { method: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (path.endsWith("/me"))
        return Response.json(
          meFixture({
            roles: ["admin"],
            allow: {
              contact: ["read"],
              consent_config: options.writable === false ? [] : ["update"],
            },
          }),
        );
      if (path.endsWith("/consent-purposes/p1")) {
        const body = request.method === "PATCH" ? await request.json() : null;
        writes.push({ method: request.method, body });
        if (options.refused)
          return Response.json(
            {
              status: 409,
              code: "consent_purpose_in_use",
              title: "Conflict",
              detail: "consent is recorded",
            },
            { status: 409 },
          );
        if (request.method === "DELETE") {
          rows = [];
          return new Response(null, { status: 204 });
        }
        rows = [{ ...purpose, label: "New label" }];
        return Response.json(rows[0]);
      }
      return Response.json({ data: rows, page: { has_more: false } });
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ConsentPurposesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { writes, user: userEvent.setup() };
}
async function open(user: ReturnType<typeof userEvent.setup>, action: string) {
  await screen.findByText(purpose.label);
  await user.click(screen.getByRole("button", { name: en["table.actions"] }));
  await user.click(screen.getByRole("button", { name: action }));
  return await screen.findByRole("dialog");
}
it("renames an unused purpose and reloads its label", async () => {
  const { writes, user } = setup();
  const dialog = await open(user, en["privacy.purposeRename"]);
  const label = within(dialog).getByRole("textbox", {
    name: en["privacy.purposeLabel"],
  });
  await user.clear(label);
  await user.type(label, " New label ");
  await user.click(
    within(dialog).getByRole("button", { name: en["record.save"] }),
  );
  expect(await screen.findByText("New label")).toBeTruthy();
  expect(writes).toEqual([{ method: "PATCH", body: { label: "New label" } }]);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
it("keeps the entered label when recorded consent refuses a rename", async () => {
  const { writes, user } = setup({ refused: true });
  const dialog = await open(user, en["privacy.purposeRename"]);
  const label = within(dialog).getByRole("textbox", {
    name: en["privacy.purposeLabel"],
  });
  await user.clear(label);
  await user.type(label, "New label");
  await user.click(
    within(dialog).getByRole("button", { name: en["record.save"] }),
  );
  expect(
    await within(dialog).findByText(en["privacy.purposeInUse"]),
  ).toBeTruthy();
  expect(label).toHaveValue("New label");
  expect(writes).toHaveLength(1);
});
it("archives an unused purpose and removes it from the choices", async () => {
  const { writes, user } = setup();
  const dialog = await open(user, en["record.archive"]);
  await user.click(
    within(dialog).getByRole("button", { name: en["record.archive"] }),
  );
  await waitFor(() => expect(screen.queryByText(purpose.label)).toBeNull());
  expect(writes).toEqual([{ method: "DELETE", body: null }]);
});
it("offers no purpose changes without the update grant", async () => {
  setup({ writable: false });
  await screen.findByText(purpose.label);
  expect(
    screen.queryByRole("button", { name: en["table.actions"] }),
  ).toBeNull();
});

it("keeps Save available and focuses a blank purpose label without writing", async () => {
  const { writes, user } = setup();
  const dialog = await open(user, en["privacy.purposeRename"]);
  const label = within(dialog).getByRole("textbox", {
    name: en["privacy.purposeLabel"],
  });
  await user.clear(label);
  const save = within(dialog).getByRole("button", { name: en["record.save"] });
  expect(save).toBeEnabled();
  await user.click(save);
  await waitFor(() => expect(label).toHaveAttribute("aria-invalid", "true"));
  expect(label).toHaveFocus();
  expect(writes).toHaveLength(0);
});
