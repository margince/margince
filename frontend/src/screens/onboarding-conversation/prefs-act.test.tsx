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

// The preferences act closes every path. What it asks is prefilled from the
// server, what it writes is only what changed, and completion is written
// only after the settings patch has landed — never before.

const settings = {
  name: "Brandt Automotive",
  timezone: "Europe/Berlin",
  base_currency: "EUR",
  base_language: "en",
  fiscal_year_start_month: 1,
  sign_in_providers: [],
  base_currency_locked: false,
  max_upload_bytes: 26214400,
};

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

function renderPrefs(admin: boolean, patches: unknown[] = []) {
  installFetchStub({
    "GET /me": meRoute(admin ? { installation_settings: ["update"] } : {}),
    "GET /installation/settings": () => jsonResponse(settings),
    "PATCH /installation/settings": (body: unknown) => {
      patches.push(body);
      return jsonResponse({ ...settings, ...(body as object) });
    },
    "GET /autonomy": () => jsonResponse(autonomy),
  });
  const state: ConversationState = {
    ...initialConversationState,
    act: "prefs",
    phase: "pf.ask",
    memberPath: !admin,
  };
  const dispatch = vi.fn();
  const persist = vi.fn(async () => true);
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
  it("prefills the reporting basis for an admin and writes nothing when Done agrees with it", async () => {
    const patches: unknown[] = [];
    const { dispatch, persist } = renderPrefs(true, patches);
    const user = userEvent.setup();

    expect(await screen.findByLabelText("Base currency")).toHaveValue("EUR");
    expect(screen.getByLabelText("Reporting timezone")).toHaveValue(
      "Europe/Berlin",
    );
    expect(
      await screen.findByText("What it may change on its own"),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Done" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "PREFS_DONE" }),
    );
    expect(patches).toEqual([]);
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "complete" }),
    );
  });

  it("patches only what changed, and only then records completion", async () => {
    const patches: unknown[] = [];
    const { dispatch, persist } = renderPrefs(true, patches);
    const user = userEvent.setup();

    const currency = await screen.findByLabelText("Base currency");
    await user.clear(currency);
    await user.type(currency, "chf");
    await user.click(screen.getByRole("button", { name: "Done" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "PREFS_DONE" }),
    );
    // Upper-cased, and the untouched timezone stays out of the patch.
    expect(patches).toEqual([{ base_currency: "CHF" }]);
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "complete" }),
    );
  });

  it("names a malformed currency beside Done and goes nowhere", async () => {
    const { dispatch, persist } = renderPrefs(true);
    const user = userEvent.setup();

    const currency = await screen.findByLabelText("Base currency");
    await user.clear(currency);
    await user.type(currency, "eu");
    await user.click(screen.getByRole("button", { name: "Done" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/three letters/);
    expect(dispatch).not.toHaveBeenCalled();
    expect(persist).not.toHaveBeenCalled();
  });

  it("asks a member only what is theirs: no reporting basis, the autonomy switches, and Done completes", async () => {
    const { dispatch, persist } = renderPrefs(false);
    const user = userEvent.setup();

    expect(
      await screen.findByText("What it may change on its own"),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Base currency")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Done" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "PREFS_DONE" }),
    );
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "complete" }),
    );
  });
});
