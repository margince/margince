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
import { armHoverIntent } from "../../../design-system/hoverintent-testing";
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
            deal={deal("d-1", "Fleet telematics rollout", {
              writable: true,
              ...over,
            })}
            openStages={[]}
            onEditDetails={() => undefined}
          />
        </RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Filing an activity onto a deal adds to it, so a deal the reader may only
// read refuses Log over its own sentence.
it("refuses Log on a deal the reader may not write", async () => {
  mount({ activity: ["read", "create"], deal: ["read"] }, { writable: false });
  const reason = await screen.findByText(en["record.attachRefused"]);
  const log = screen.getByRole("button", { name: en["log.title"] });
  expect(log).toHaveProperty("disabled", true);
  expect(log.getAttribute("aria-describedby")).toBe(reason.id);
});

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
    ).toBe("Your role can only read. Ask an administrator for another role.");
  });

  it("names the refusal in its tooltip, described once by the header's sentence", async () => {
    const user = userEvent.setup();
    armHoverIntent();
    mount({ deal: ["read"] });
    const why = en["record.roleReadsOnly"];

    const email = screen.getByRole("button", { name: "Email" });
    const sentence = await screen.findByText(why);
    await expect.poll(() => email.hasAttribute("disabled")).toBe(true);
    expect(email.getAttribute("aria-describedby")).toBe(sentence.id);
    expect(screen.getAllByText(why)).toHaveLength(1);

    const host = email.closest(".icon-action");
    if (!(host instanceof HTMLElement)) {
      throw new Error("the Email verb lost its tooltip host");
    }
    await user.hover(host);
    expect((await screen.findByRole("tooltip")).textContent).toBe(
      `Email. ${why}`,
    );
    for (const [name, label] of [
      ["Log activity", en["log.title"]],
      [en["log.kindCall"], en["log.kindCall"]],
    ]) {
      expect(screen.getByRole("button", { name }).getAttribute("title")).toBe(
        `${label}. ${why}`,
      );
    }
  });

  it("is live with the activity grant", async () => {
    mount({ deal: ["read"], activity: ["create"] });

    const email = await screen.findByRole("button", { name: "Email" });
    await expect.poll(() => email.hasAttribute("disabled")).toBe(false);
  });
});

const meet = () =>
  screen.getByRole("button", {
    name: `${en["record.verb.meet"]}, ${en["scheduling.bookContact"]}`,
  });

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
    expect(meet()).toHaveAttribute(
      "title",
      `${en["scheduling.bookContact"]}. ${en["deal.meetNeedsCompany"]}`,
    );
  });

  it("says the company is not shared, not missing, when it is withheld", async () => {
    mount(grants, { company_id: null, masked_fields: ["company_id"] });

    await waitFor(() =>
      expect(meet()).toHaveAccessibleDescription(
        en["deal.meetCompanyWithheld"],
      ),
    );
    expect(meet()).toBeDisabled();
    expect(meet()).not.toHaveAccessibleDescription(en["deal.meetNeedsCompany"]);
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
    expect(screen.getByRole("button", { name: "Email" })).toHaveAttribute(
      "aria-keyshortcuts",
      "m",
    );

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
      "Your role can only read. Ask an administrator for another role.",
    );

    await user.keyboard("ntm");
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

// The way back from an archive is on the page's band, beside the sentence
// that says the deal is archived. So the menu offers neither verb then.
describe("an archived deal's menu", () => {
  async function openMore() {
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: en["record.moreActions"] }),
    );
    await screen.findByRole("button", { name: en["log.addTask"] });
  }

  it("offers neither Archive deal nor Restore deal on an archived deal", async () => {
    mount(
      { deal: ["read", "update", "delete"] },
      { archived_at: "2026-11-01T09:00:00Z", writable: false },
    );
    await openMore();

    expect(
      screen.queryByRole("button", { name: en["deal.archive"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["deal.restore"] }),
    ).toBeNull();
  });

  it("offers Archive deal, not Restore deal, on a live deal", async () => {
    mount({ deal: ["read", "update", "delete"] }, { writable: true });
    await openMore();

    expect(
      await screen.findByRole("button", { name: en["deal.archive"] }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["deal.restore"] }),
    ).toBeNull();
  });
});
