// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { GrantSpec } from "../app/mefixture";
import { en } from "../i18n/en";
import { AutomationsAdmin } from "./automations";
import {
  AUTOMATION_CATALOG,
  configuredAutomations,
} from "./automations.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const OPERATOR: GrantSpec = {
  automation: ["create", "read", "update", "delete"],
};
const RECORDED_AT = Date.parse("2026-09-01T12:00:00Z");

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  globalThis.localStorage.clear();
});

function mount(allow: GrantSpec = OPERATOR) {
  installFetchStub({
    "GET /me": meRoute(allow),
    "GET /automations/catalog": () =>
      jsonResponse({ data: AUTOMATION_CATALOG }),
    "GET /automations": () =>
      jsonResponse({
        data: configuredAutomations(RECORDED_AT),
        page: { next_cursor: null },
      }),
  });
  render(
    <StoryProviders>
      <AutomationsAdmin />
    </StoryProviders>,
  );
}

const row = (id: string) => screen.findByTestId(`automation-${id}`);

describe("the configured automations table", () => {
  it("reads each rule's last run as an outcome word, and Never when it has none", async () => {
    mount();
    expect(await row("au-1")).toHaveTextContent(en["auto.runs.outcomeFired"]);
    const failed = within(await row("au-2")).getByText(
      en["auto.runs.outcomeFailed"],
    );
    expect(failed.closest(".badge")?.className).toContain("badge-danger");
    const blocked = within(await row("au-3")).getByText(
      en["auto.runs.outcomeBlocked"],
    );
    expect(blocked.closest(".badge")?.className).toContain("badge-warning");
    expect(await row("au-4")).toHaveTextContent(en["auto.runs.outcomeQueued"]);
    expect(
      within(await row("au-5")).getByText(en["auto.lastRunNever"]),
    ).toBeInTheDocument();
    const at = (await row("au-1")).querySelector("time");
    expect(at?.getAttribute("dateTime")).toBe(
      configuredAutomations(RECORDED_AT)[0].last_run_at,
    );
  });

  it("puts the 30-day run count in an end-aligned column, grouped for the reader", async () => {
    mount();
    const heading = await screen.findByRole("columnheader", {
      name: en["auto.colRuns30"],
    });
    expect(heading.className).toContain("datatable-end");
    const figure = within(await row("au-4")).getByText("1,240");
    expect(figure.closest("td")?.className).toContain("datatable-end");
    expect(within(await row("au-5")).getByText("0")).toBeInTheDocument();
  });

  it("captions a rule the system paused with why, and leaves one paused by hand bare", async () => {
    mount();
    const paused = await row("au-6");
    expect(
      within(paused).getByRole("button", {
        name: en["auto.pausedShort.listArchived"],
      }),
    ).toBeInTheDocument();
    expect(await row("au-5")).not.toHaveTextContent(/Paused ·/);
  });

  it("says what a rule does in words, never in its key or raw parameters", async () => {
    mount();
    const first = await row("au-1");
    expect(first).toHaveTextContent("Quiet accounts follow-up");
    expect(first).toHaveTextContent(
      `${en["auto.trigger.noActivity"]}: ${en["auto.action.createTask"]}`,
    );
    for (const raw of ["no_activity_reminder", "days=", "create_task"]) {
      expect(first).not.toHaveTextContent(raw);
    }
    expect(await row("au-6")).not.toHaveTextContent(/list_id|list\.evaluated/);
  });
});

describe("the starter library", () => {
  it("opens a template's create dialog from a press anywhere on its row", async () => {
    const user = userEvent.setup();
    mount();
    const template = await screen.findByTestId("template-renewal_reminder");
    expect(template).not.toHaveTextContent(/clock:|create_task|->/);
    expect(template).toHaveTextContent(en["auto.trigger.renewal"]);
    await user.click(
      within(template).getByText(AUTOMATION_CATALOG[1].description ?? ""),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByRole("heading", { name: "Renewal reminder" }),
    ).toBeInTheDocument();
  });

  it("reaches the same verb from the keyboard, and Escape puts it away", async () => {
    const user = userEvent.setup();
    mount();
    const template = await screen.findByTestId("template-post_meeting_recap");
    within(template).getByRole("button", { name: en["auto.use"] }).focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("stays readable without the create grant, but no row opens", async () => {
    const user = userEvent.setup();
    mount({ automation: ["read"] });
    const template = await screen.findByTestId("template-renewal_reminder");
    expect(template.className).not.toContain("rowlink");
    expect(within(template).queryByRole("button")).toBeNull();
    await user.click(within(template).getByText("Renewal reminder"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
