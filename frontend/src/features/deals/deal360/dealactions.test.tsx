/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { useShortcutDispatcher } from "../../../lib/shortcuts";
import { deal } from "../../../testing/brief.fixtures";
import { RecordShell } from "../../../testing/recordshell.testkit";
import { stubWithSession } from "../../../testing/story-utils";
import { DealActions } from "./dealactions";

type Deal = components["schemas"]["Deal"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(
  allow: Parameters<typeof stubWithSession>[1],
  over: Partial<Deal> = {},
) {
  stubWithSession({}, allow);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <RecordShell>
          <DealActions
            deal={deal("d-1", "Fleet telematics rollout", over)}
            openStages={[]}
            onEditDetails={() => undefined}
          />
        </RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the deal header's Email verb", () => {
  it("is refused without the activity grant, over the same sentence as Log", async () => {
    mount({ deal: ["read"] });

    const log = await screen.findByRole("button", { name: "Log activity" });
    const email = screen.getByRole("button", { name: "Email" });
    await expect
      .poll(() => email.getAttribute("aria-describedby"))
      .toBeTruthy();
    expect(email.hasAttribute("disabled")).toBe(true);
    expect(email.getAttribute("aria-describedby")).toBe(
      log.getAttribute("aria-describedby"),
    );
    expect(
      document.getElementById(email.getAttribute("aria-describedby") ?? "")
        ?.textContent,
    ).toBe("You do not have permission to log activities on this record.");
  });

  it("is live with the activity grant", async () => {
    mount({ deal: ["read"], activity: ["create"] });

    const email = await screen.findByRole("button", { name: "Email" });
    await expect.poll(() => email.hasAttribute("disabled")).toBe(false);
  });
});

const meet = () =>
  screen.getByRole("button", { name: en["scheduling.bookContact"] });

describe("the deal header's Meet verb", () => {
  const grants: Parameters<typeof stubWithSession>[1] = {
    deal: ["read", "update"],
    activity: ["create"],
  };

  it("says why Meet is off on a deal with no company", async () => {
    mount(grants, { company_id: null });

    await waitFor(() =>
      expect(meet()).toHaveAccessibleDescription(en["deal.meetNeedsCompany"]),
    );
    expect(meet()).toBeDisabled();
  });

  it("offers Meet with no refusal on a deal with a company", async () => {
    mount(grants, { company_id: "01a02000-0000-7000-8000-000000000001" });

    await waitFor(() => expect(meet()).toBeEnabled());
    expect(meet()).not.toHaveAccessibleDescription(en["deal.meetNeedsCompany"]);
  });
});

describe("the deal's record keys", () => {
  const grants: Parameters<typeof stubWithSession>[1] = {
    deal: ["read", "update"],
    activity: ["create"],
  };

  async function mountLive() {
    renderHook(() => useShortcutDispatcher());
    mount(grants);
    const log = await screen.findByRole("button", { name: en["log.title"] });
    await waitFor(() => expect(log).toBeEnabled());
    return log;
  }

  it.each([
    ["n", en["log.title"]],
    ["t", en["log.addTask"]],
  ])("open on %s the drawer its verb opens: %s", async (key, verb) => {
    const user = userEvent.setup();
    await mountLive();

    await user.keyboard(key);
    const drawer = await screen.findByRole("dialog");
    expect(within(drawer).getByRole("heading", { name: verb })).toBeTruthy();
  });

  it("opens on M the composer the Email verb opens", async () => {
    const user = userEvent.setup();
    const log = await mountLive();
    expect(log).toHaveAttribute("aria-keyshortcuts", "n");

    await user.keyboard("m");
    expect(
      await screen.findByRole("dialog", { name: en["compose.newEmail"] }),
    ).toBeTruthy();
  });

  it("does nothing without the activity grant", async () => {
    const user = userEvent.setup();
    renderHook(() => useShortcutDispatcher());
    mount({ deal: ["read"] });
    await screen.findByText(
      "You do not have permission to log activities on this record.",
    );

    await user.keyboard("ntm");
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
