/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { installFetchStub, jsonResponse, meRoute } from "../story-utils";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";
import { PrefsAct } from "./prefs-act";

// The preferences act closes every path. What it asks is the reader's own
// autonomy switches, and completion is a server fact before the handoff plays.

const autonomy = {
  data: [
    {
      kind: "close_date_correction",
      mode: "manual",
      approved_clean: 0,
      approved_edited: 0,
      rejected: 0,
    },
  ],
};

function renderPrefs(admin: boolean, persisted = true) {
  installFetchStub({
    "GET /me": meRoute(admin ? { installation_settings: ["update"] } : {}),
    "GET /autonomy": () => jsonResponse(autonomy),
  });
  const state: ConversationState = {
    ...initialConversationState,
    act: "prefs",
    phase: "pf.ask",
    memberPath: !admin,
  };
  const dispatch = vi.fn();
  const persist = vi.fn(async () => persisted);
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <PrefsAct state={state} dispatch={dispatch} persist={persist} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { dispatch, persist };
}

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("PrefsAct", () => {
  it("asks what the agent may change on its own, and Done records completion", async () => {
    const { dispatch, persist } = renderPrefs(true);
    const user = userEvent.setup();

    expect(
      await screen.findByText("What it may change on its own"),
    ).toBeInTheDocument();
    // The reporting basis was the basis act's question, right after the
    // company; it is never asked twice.
    expect(screen.queryByLabelText("Base currency")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Done" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "PREFS_DONE" }),
    );
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "complete" }),
    );
  });

  it("asks a member the same, on their own three-stop rail", async () => {
    const { dispatch, persist } = renderPrefs(false);
    const user = userEvent.setup();

    expect(
      await screen.findByText("What it may change on its own"),
    ).toBeInTheDocument();
    expect(screen.getByText("Step 3 of 3 · Preferences")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Done" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "PREFS_DONE" }),
    );
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "complete" }),
    );
  });

  it("says so and stays when completion could not be written", async () => {
    const { dispatch, persist } = renderPrefs(true, false);
    const user = userEvent.setup();

    await screen.findByText("What it may change on its own");
    await user.click(screen.getByRole("button", { name: "Done" }));

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(persist).toHaveBeenCalledTimes(1);
    expect(dispatch).not.toHaveBeenCalled();
  });
});
