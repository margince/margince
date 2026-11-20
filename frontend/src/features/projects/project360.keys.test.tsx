// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  renderHook,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { useShortcutDispatcher } from "../../lib/shortcuts";
import { project, project360 } from "../../testing/projects.fixtures";
import { RecordShell } from "../../testing/recordshell.testkit";
import { ProjectScreen } from "./project360";
import { projectsBackend } from "./projects.testing";

// The project page's single keys: N opens the log, T a task, as on every
// record page; an archived project takes neither.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the project's record keys", () => {
  async function mountProject(view = project360()) {
    projectsBackend({ view });
    renderHook(() => useShortcutDispatcher());
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });
    return screen.getByRole("button", { name: en["log.title"] });
  }

  it.each([
    ["n", en["log.title"]],
    ["t", en["log.addTask"]],
  ])("open on %s the drawer its verb opens: %s", async (key, verb) => {
    const user = userEvent.setup();
    const log = await mountProject();
    await waitFor(() => expect(log.hasAttribute("disabled")).toBe(false));
    expect(log.getAttribute("aria-keyshortcuts")).toBe("n");

    await user.keyboard(key);
    const drawer = await screen.findByRole("dialog");
    expect(within(drawer).getByRole("heading", { name: verb })).toBeTruthy();
  });

  it("bind no M: a project has no Email verb", async () => {
    const user = userEvent.setup();
    const log = await mountProject();
    await waitFor(() => expect(log.hasAttribute("disabled")).toBe(false));

    await user.keyboard("m");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("open the log on a project this caller may not change", async () => {
    const user = userEvent.setup();
    const log = await mountProject(
      project360({
        project: project({ owner_id: "u-someone-else", writable: false }),
      }),
    );
    await screen.findByText(/You cannot edit this project/);
    await waitFor(() => expect(log.hasAttribute("disabled")).toBe(false));

    await user.keyboard("n");
    expect(await screen.findByRole("dialog")).toBeTruthy();
  });

  it("do nothing on an archived project", async () => {
    const user = userEvent.setup();
    await mountProject(
      project360({
        project: project({ archived_at: "2026-01-02T00:00:00Z" }),
      }),
    );
    await screen.findByText(en["project.archivedReadOnly"]);

    await user.keyboard("nt");
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
