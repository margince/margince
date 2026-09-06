/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { installFetchStub, jsonResponse, meRoute } from "../story-utils";
import { BasisAct } from "./basis-act";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// The basis act asks the installation's reporting basis right after the
// company, and beside it what the agent may change on its own. The basis is
// prefilled from the server, what it writes is only what changed, and the act
// moves on only after the settings patch has landed — never before. The
// autonomy switches are the reader's own and write themselves as they move,
// so Continue never waits on them.

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

function renderBasis(admin: boolean, patches: unknown[] = []) {
  installFetchStub({
    "GET /me": meRoute(admin ? { installation_settings: ["update"] } : {}),
    "GET /autonomy": () => jsonResponse(autonomy),
    "GET /installation/settings": () => jsonResponse(settings),
    "PATCH /installation/settings": (body: unknown) => {
      patches.push(body);
      return jsonResponse({ ...settings, ...(body as object) });
    },
  });
  const state: ConversationState = {
    ...initialConversationState,
    act: "basis",
    phase: "bs.ask",
  };
  const dispatch = vi.fn();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BasisAct state={state} dispatch={dispatch} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { dispatch };
}

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("BasisAct", () => {
  it("prefills the reporting basis and writes nothing when Continue agrees with it", async () => {
    const patches: unknown[] = [];
    const { dispatch } = renderBasis(true, patches);
    const user = userEvent.setup();

    expect(await screen.findByLabelText("Base currency")).toHaveValue("EUR");
    expect(screen.getByLabelText("Reporting timezone")).toHaveValue(
      "Europe/Berlin",
    );
    // Third of five: the stop sits between the confirmation and the voice.
    expect(screen.getByText("Step 3 of 5 · Basis")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "BASIS_DONE" }),
    );
    expect(patches).toEqual([]);
  });

  it("patches only what changed, and only then moves on", async () => {
    const patches: unknown[] = [];
    const { dispatch } = renderBasis(true, patches);
    const user = userEvent.setup();

    const currency = await screen.findByLabelText("Base currency");
    await user.clear(currency);
    await user.type(currency, "chf");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "BASIS_DONE" }),
    );
    // Upper-cased, and the untouched timezone stays out of the patch.
    expect(patches).toEqual([{ base_currency: "CHF" }]);
  });

  it("names a malformed currency beside Continue and goes nowhere", async () => {
    const { dispatch } = renderBasis(true);
    const user = userEvent.setup();

    const currency = await screen.findByLabelText("Base currency");
    await user.clear(currency);
    await user.type(currency, "eu");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/three letters/);
    expect(dispatch).not.toHaveBeenCalled();
  });

  it("holds Continue until the reporting basis has been read", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    installFetchStub({
      "GET /me": meRoute({ installation_settings: ["update"] }),
      "GET /autonomy": () => jsonResponse(autonomy),
      "GET /installation/settings": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    const state: ConversationState = {
      ...initialConversationState,
      act: "basis",
      phase: "bs.ask",
    };
    const dispatch = vi.fn();
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <BasisAct state={state} dispatch={dispatch} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();

    // Nothing stored yet: a Continue that pressed would settle the basis on
    // values nobody has seen, so it does not press. (The grant itself arrives
    // with the session probe, which the shell has long settled by the time
    // this act mounts; here it is fetched, hence the wait.)
    const onward = await screen.findByRole("button", { name: "Continue" });
    await waitFor(() => expect(onward).toBeDisabled());
    await user.click(onward);
    expect(dispatch).not.toHaveBeenCalled();

    deferred.resolve?.(jsonResponse(settings));
    await waitFor(() => expect(onward).not.toBeDisabled());
    await user.click(onward);
    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "BASIS_DONE" }),
    );
  });

  it("asks what the agent may change on its own beside the basis, and the switches write themselves", async () => {
    const patches: unknown[] = [];
    const choices: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ installation_settings: ["update"] }),
      "GET /installation/settings": () => jsonResponse(settings),
      "PATCH /installation/settings": (body: unknown) => {
        patches.push(body);
        return jsonResponse(settings);
      },
      "GET /autonomy": () => jsonResponse(autonomy),
      "PATCH /autonomy": (body: unknown) => {
        choices.push(body);
        return jsonResponse({ ...autonomy.data[0], mode: "auto" });
      },
    });
    const state: ConversationState = {
      ...initialConversationState,
      act: "basis",
      phase: "bs.ask",
    };
    const dispatch = vi.fn();
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <BasisAct state={state} dispatch={dispatch} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();

    expect(
      await screen.findByText("What it may change on its own"),
    ).toBeInTheDocument();
    const toggle = screen.getByTestId("autonomy-toggle-close_date_correction");
    expect(toggle).not.toBeChecked();

    // The switch records itself the moment it moves; Continue has nothing
    // left to write for it.
    await user.click(toggle);
    await waitFor(() => expect(choices).toHaveLength(1));
    expect(choices[0]).toMatchObject({
      kind: "close_date_correction",
      auto: true,
    });

    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "BASIS_DONE" }),
    );
    expect(patches).toHaveLength(0);
  });

  it("asks the reporting basis of nobody who may not change it, still asks the autonomy, and continues", async () => {
    const { dispatch } = renderBasis(false);
    const user = userEvent.setup();

    expect(
      await screen.findByRole("heading", { name: "First, the basis." }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Base currency")).toBeNull();
    // The autonomy is the reader's own, whatever grants they hold.
    expect(
      await screen.findByText("What it may change on its own"),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "BASIS_DONE" }),
    );
  });
});
