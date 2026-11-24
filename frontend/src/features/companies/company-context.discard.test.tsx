/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UnsavedGuard, useHasUnsavedChanges } from "../../components/unsaved";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { CompanyContextCard } from "./company-context";

// Edit business context: Cancel throws the draft away, and Escape over a
// changed draft asks first, as every settings dialog does.

const COMPANY = {
  company_id: "00000000-0000-4000-8000-000000000010",
  display_name: "Acme",
  website: "acme.test",
  offer_summary: "We sell field service software",
  icp: "Operations leads at mid-market installers",
};

const writes: string[] = [];

function backend() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request =
      input instanceof Request ? input : new Request(String(input), init);
    const path = new URL(request.url).pathname;
    if (request.method !== "GET") writes.push(`${request.method} ${path}`);
    if (path === "/v1/me") {
      return Response.json(
        meFixture({ allow: { company: ["create", "read", "update"] } }),
      );
    }
    if (path === "/v1/company/context/capabilities") {
      return Response.json({
        rollout: "onboarding",
        read_enabled: true,
        tasks_enabled: true,
        onboarding_enabled: true,
      });
    }
    if (path === "/v1/company") return Response.json(COMPANY);
    throw new Error(`unstubbed request: ${request.method} ${path}`);
  });
}

function UnsavedProbe() {
  return (
    <span data-testid="unsaved-probe">
      {useHasUnsavedChanges() ? "dirty" : "clean"}
    </span>
  );
}

async function renderCard() {
  vi.stubGlobal("fetch", backend());
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <UnsavedGuard address="settings" onKeep={() => undefined}>
          {() => (
            <>
              <CompanyContextCard />
              <UnsavedProbe />
            </>
          )}
        </UnsavedGuard>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await screen.findByRole("button", { name: "Edit business context" });
}

// The card is on screen once renderCard resolves, so opening it waits on nothing.
async function openEditor(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    screen.getByRole("button", { name: "Edit business context" }),
  );
  return screen.getByRole("dialog");
}

const companyName = (dialog: HTMLElement) =>
  within(dialog).getByRole<HTMLInputElement>("textbox", {
    name: "Company name",
  });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  writes.length = 0;
});

describe("Edit business context", () => {
  it("discards the draft on Cancel, so reopening shows the saved profile and leaving asks nothing", async () => {
    const user = userEvent.setup();
    await renderCard();

    let dialog = await openEditor(user);
    await user.clear(companyName(dialog));
    await user.type(companyName(dialog), "Acme Field Services");
    expect(screen.getByTestId("unsaved-probe").textContent).toBe("dirty");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByTestId("unsaved-probe").textContent).toBe("clean");
    dialog = await openEditor(user);
    expect(companyName(dialog).value).toBe("Acme");
    expect(writes).toEqual([]);
  });

  it("asks before Escape throws away a changed draft, and closes at once when nothing changed", async () => {
    const user = userEvent.setup();
    await renderCard();

    await openEditor(user);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();

    const dialog = await openEditor(user);
    await user.type(companyName(dialog), " GmbH");
    await user.keyboard("{Escape}");
    const question = await screen.findByRole("dialog", {
      name: "Discard unsaved changes?",
    });
    await user.click(
      within(question).getByRole("button", { name: "Discard changes" }),
    );

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(companyName(await openEditor(user)).value).toBe("Acme");
  });
});
