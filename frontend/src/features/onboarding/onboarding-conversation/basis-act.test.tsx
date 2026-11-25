/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { type Locale, LocaleProvider } from "../../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../../testing/story-utils";
import { BasisAct } from "./basis-act";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// Setup confirms the installation reporting basis without asking for personal automation settings.

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

function renderBasis(admin: boolean, patches: unknown[] = []) {
  installFetchStub({
    "GET /me": meRoute(admin ? { installation_settings: ["update"] } : {}),
    "GET /installation/settings": () => jsonResponse(settings),
    "PATCH /installation/settings": (body: unknown) => {
      patches.push(body);
      return jsonResponse({ ...settings, ...(body as object) });
    },
  });
  return mountBasis("en");
}

// The PATCH is refused with these field faults. `lockAfter` is a lock a rate
// set meanwhile, which the read after the refusal reports.
function renderRefused(
  errors: unknown[],
  locale: Locale,
  lockAfter?: components["schemas"]["BaseCurrencyLock"],
) {
  let refused = false;
  installFetchStub({
    "GET /me": meRoute({ installation_settings: ["update"] }),
    "GET /installation/settings": () =>
      jsonResponse(
        refused && lockAfter
          ? {
              ...settings,
              base_currency_locked: true,
              base_currency_lock: lockAfter,
            }
          : settings,
      ),
    "PATCH /installation/settings": () => {
      refused = true;
      return jsonResponse(
        {
          code: "validation_error",
          detail: "The installation could not be saved.",
          details: { errors },
        },
        422,
      );
    },
  });
  return mountBasis(locale);
}

function mountBasis(locale: Locale) {
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
      <LocaleProvider initial={locale}>
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
    expect(screen.getByLabelText("Company time zone")).toHaveValue(
      "Europe/Berlin",
    );
    // Third of five: the stop sits between the confirmation and the voice.
    expect(screen.getByText("Step 3 of 5 · Basis")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "BASIS_DONE" }),
    );
    expect(patches).toEqual([]);
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
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

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /3-letter currency code/,
    );
    expect(dispatch).not.toHaveBeenCalled();
  });

  it("words a currency frozen after the step loaded as its lock, once, beside the field", async () => {
    const { dispatch } = renderRefused(
      [
        {
          field: "installation.base_currency",
          code: "setting_frozen",
          message: "2 rate(s) in the sheet convert into EUR",
          params: { reason: "rate_sheet", count: "2", base_currency: "EUR" },
        },
      ],
      "de",
      { reason: "rate_sheet", count: 2, base_currency: "EUR" },
    );
    const user = userEvent.setup();

    const currency = await screen.findByLabelText("Basiswährung");
    await user.clear(currency);
    await user.type(currency, "chf");
    await user.click(screen.getByRole("button", { name: "Weiter" }));

    const sentence =
      "2 Wechselkurse rechnen in EUR um. Entferne zuerst die Wechselkurse, um die Basiswährung zu ändern.";
    const refusal = await screen.findByText(sentence);
    expect(currency).toHaveAttribute("aria-invalid", "true");
    expect(currency.getAttribute("aria-describedby")).toContain(refusal.id);
    // The read after the refusal locks the field, and its hint holds the same sentence.
    await waitFor(() => expect(currency).toBeDisabled());
    expect(screen.getAllByText(sentence)).toHaveLength(1);
    expect(screen.queryByText(/convert into EUR/)).toBeNull();
    expect(dispatch).not.toHaveBeenCalled();
  });

  it("puts a refused time zone beside its field, not at the foot", async () => {
    renderRefused(
      [
        {
          field: "installation.timezone",
          code: "setting_invalid",
          message: '"Mars/Olympus" is not an IANA zone name this server knows',
        },
      ],
      "en",
    );
    const user = userEvent.setup();

    const zone = await screen.findByLabelText("Company time zone");
    await user.clear(zone);
    await user.type(zone, "Mars/Olympus");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    const refusal = await screen.findByText(/not an IANA zone name/);
    expect(zone).toHaveAttribute("aria-invalid", "true");
    expect(zone.getAttribute("aria-describedby")).toContain(refusal.id);
    expect(screen.getAllByRole("alert")).toHaveLength(1);
  });

  it("says a refusal of a field the step does not draw at the foot", async () => {
    renderRefused(
      [
        {
          field: "installation.base_language",
          code: "setting_invalid",
          message: "Not a language this server speaks.",
        },
      ],
      "en",
    );
    const user = userEvent.setup();

    const zone = await screen.findByLabelText("Company time zone");
    await user.type(zone, "x");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The installation could not be saved.",
    );
    expect(zone).not.toHaveAttribute("aria-invalid");
  });

  it("holds Continue until the reporting basis has been read", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    installFetchStub({
      "GET /me": meRoute({ installation_settings: ["update"] }),
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

  it("lets a reader without reporting permissions continue", async () => {
    const { dispatch } = renderBasis(false);
    const user = userEvent.setup();

    expect(
      await screen.findByRole("heading", { name: "Set the reporting basis" }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Base currency")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "BASIS_DONE" }),
    );
  });
});
