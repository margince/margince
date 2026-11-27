// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/company.fixtures";
import { project, project360 } from "../../testing/projects.fixtures";
import { RecordShell } from "../../testing/recordshell.testkit";
import { ProjectScreen } from "./project360";
import { projectsBackend } from "./projects.testing";

// A phase move from the project page: the close dialog holds its Confirm until
// a reason is given, then posts `{to_phase: "closed", reason}` under
// the project's version. Any other move needs no reason.

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

describe("a phase move from the project page", () => {
  it("closing asks for a reason, holds Confirm until it is given, and posts the move", async () => {
    const user = userEvent.setup();
    let posted: { body: unknown; ifMatch: string | null } | null = null;
    projectsBackend({
      respond: async (url, method, request) => {
        if (method === "POST" && url.endsWith("/projects/pr-1/advance")) {
          posted = {
            body: JSON.parse(await request.text()),
            ifMatch: request.headers.get("If-Match"),
          };
          return jsonResponse(project({ phase: "closed" }));
        }
        return null;
      },
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });

    await user.click(screen.getByTestId("project-step-closed"));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByRole("heading", { name: "Close project" }),
    ).toBeTruthy();
    const confirm = within(dialog).getByRole("button", {
      name: "Close project",
    });
    await user.click(confirm);
    expect(posted).toBeFalsy();
    expect(within(dialog).getByText("Reason is required.")).toBeTruthy();
    expect(document.activeElement).toBe(
      within(dialog).getByLabelText("Reason *"),
    );

    await user.type(
      within(dialog).getByLabelText("Reason *"),
      "Delivered and signed off",
    );
    await user.click(confirm);

    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toEqual({
      body: { to_phase: "closed", reason: "Delivered and signed off" },
      ifMatch: "3",
    });
  });

  it("a move that is not a close needs no reason", async () => {
    const user = userEvent.setup();
    let posted: unknown = null;
    projectsBackend({
      respond: async (url, method, request) => {
        if (method === "POST" && url.endsWith("/advance")) {
          posted = JSON.parse(await request.text());
          return jsonResponse(project({ phase: "pursuing" }));
        }
        return null;
      },
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });
    await user.click(screen.getByTestId("project-step-pursuing"));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Move" }));
    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toEqual({ to_phase: "pursuing", reason: null });
  });

  it("shows the new phase from the move's answer and closes the dialog only once the page is read again", async () => {
    const user = userEvent.setup();
    let moved = false;
    let releaseReread: (response: Response) => void = () => {};
    projectsBackend({
      respond: async (url, method) => {
        if (method === "POST" && url.endsWith("/projects/pr-1/advance")) {
          moved = true;
          return jsonResponse(project({ phase: "pursuing" }));
        }
        if (moved && method === "GET" && url.endsWith("/projects/pr-1/360")) {
          return new Promise<Response>((resolve) => {
            releaseReread = resolve;
          });
        }
        return null;
      },
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });
    await user.click(screen.getByTestId("project-step-pursuing"));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Move" }));

    await waitFor(() => {
      const here = [...document.querySelectorAll("[aria-current='step']")];
      expect(
        here.some((node) => node.textContent?.startsWith("Pursuing")),
      ).toBe(true);
    });
    expect(screen.queryByRole("dialog")).toBeTruthy();

    releaseReread(
      jsonResponse(project360({ project: project({ phase: "pursuing" }) })),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});
