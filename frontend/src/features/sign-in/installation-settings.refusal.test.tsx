/** @vitest-environment happy-dom */
import { QueryClient } from "@tanstack/react-query";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import { InstallationSettingsCard } from "./installation-settings";
import {
  backendFor,
  openFrom,
  render,
  SETTINGS_EDITOR,
} from "./installation-settings.testkit";

// A refused save of the installation dialog: each refusal on its field or at
// the foot, and a currency locked meanwhile.

// The PATCH carries the currency, so it is refused, and the read after it
// reports the lock a rate set meanwhile.
const FROZEN = {
  refuse: [
    {
      field: "installation.base_currency",
      code: "setting_frozen",
      message: "3 deal(s) have already frozen a conversion rate against it",
      params: { reason: "rates_frozen", count: "3" },
    },
  ],
  lockAfterRefusal: { reason: "rates_frozen", count: 3 },
} as const;

const FROZEN_SENTENCE =
  "Für 3 Einträge ist bereits jeweils ein Wechselkurs zu dieser Währung festgeschrieben, daher ist sie nicht mehr änderbar.";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a refused installation save", () => {
  it("puts a refused value's reason on the field it is about", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backendFor(SETTINGS_EDITOR, {
      refuse: [
        {
          field: "timezone",
          code: "setting_invalid",
          message: "Not an IANA time zone.",
        },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<InstallationSettingsCard />);

    const dialog = await openFrom(user, /edit company time zone/i);
    const zone = within(dialog).getByLabelText(/company time zone/i);
    await user.clear(zone);
    await user.type(zone, "Europe/Berlim");
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }));

    const refusal = await screen.findByText("Not an IANA time zone.");
    // The control names the refusal, so a screen reader hears it on the input.
    expect(zone.getAttribute("aria-invalid")).toBe("true");
    expect(refusal.id).not.toBe("");
    expect(zone.getAttribute("aria-describedby") ?? "").toContain(refusal.id);
    // A refused save keeps the form open, with the edit and the reason for it.
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("marks the field a server refusal names by its setting key", async () => {
    const user = userEvent.setup();
    const sentence =
      '"Mars/Olympus" is not an IANA zone name this server knows — use one like Europe/Berlin';
    const { fetchMock } = backendFor(SETTINGS_EDITOR, {
      refuse: [
        {
          field: "installation.timezone",
          code: "setting_invalid",
          message: sentence,
        },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<InstallationSettingsCard />);

    const dialog = await openFrom(user, /edit company time zone/i);
    const zone = within(dialog).getByLabelText(/company time zone/i);
    await user.clear(zone);
    await user.type(zone, "Mars/Olympus");
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }));

    await within(dialog).findByText(/use one like Europe\/Berlin/);
    expect(zone.getAttribute("aria-invalid")).toBe("true");
  });

  it("shows a refusal naming a field the dialog does not draw", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backendFor(SETTINGS_EDITOR, {
      refuse: [
        {
          field: "installation.forecast_likely_threshold",
          code: "setting_invalid",
          message: "Threshold out of range.",
        },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<InstallationSettingsCard />);

    const dialog = await openFrom(user, /edit company time zone/i);
    const zone = within(dialog).getByLabelText(/company time zone/i);
    await user.type(zone, "x");
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }));

    await within(dialog).findByText(
      englishCopy["installationSettings.saveFailed"],
    );
    expect(dialog.querySelector('[aria-invalid="true"]')).toBeNull();
  });

  it("words a currency frozen after the page loaded as its lock, once, then saves the rest", async () => {
    const user = userEvent.setup();
    let release = () => {};
    const hold = new Promise<void>((resolve) => {
      release = resolve;
    });
    const { fetchMock, patch } = backendFor(SETTINGS_EDITOR, {
      ...FROZEN,
      hold,
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<InstallationSettingsCard />, undefined, "de");

    const dialog = await openFrom(user, /basiswährung bearbeiten/i);
    const currency = within(dialog).getByLabelText(
      "Basiswährung",
    ) as HTMLInputElement;
    await user.clear(currency);
    await user.type(currency, "CHF");
    const zone = within(dialog).getByLabelText("Zeitzone des Unternehmens");
    await user.clear(zone);
    await user.type(zone, "Europe/Vienna");
    await user.click(within(dialog).getByRole("button", { name: "Speichern" }));

    const refusal = await within(dialog).findByText(FROZEN_SENTENCE);
    expect(refusal.className).toContain("field-error");
    expect(currency.getAttribute("aria-invalid")).toBe("true");
    expect(within(dialog).queryByText(/frozen a conversion rate/)).toBeNull();
    // The read after the refusal locks the field, which drops the typed code.
    await waitFor(() => expect(currency.disabled).toBe(true));
    expect(within(dialog).getAllByText(FROZEN_SENTENCE)).toHaveLength(1);
    expect(currency.value).toBe("EUR");

    const save = within(dialog).getByRole("button", { name: "Speichern" });
    await user.click(save);
    await waitFor(() => expect(save.getAttribute("aria-busy")).toBe("true"));
    expect(within(dialog).getAllByText(FROZEN_SENTENCE)).toHaveLength(1);
    release();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(patch()).toEqual({ timezone: "Europe/Vienna" });
  });

  it("shows the stored currency, not the refused one, once the lock lifts again", async () => {
    const user = userEvent.setup();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { fetchMock, unlock } = backendFor(SETTINGS_EDITOR, FROZEN);
    vi.stubGlobal("fetch", fetchMock);
    render(<InstallationSettingsCard />, client, "de");

    const dialog = await openFrom(user, /basiswährung bearbeiten/i);
    const currency = within(dialog).getByLabelText(
      "Basiswährung",
    ) as HTMLInputElement;
    await user.clear(currency);
    await user.type(currency, "CHF");
    await user.click(within(dialog).getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(currency.disabled).toBe(true));

    unlock();
    await client.invalidateQueries({ queryKey: ["installation-settings"] });
    await waitFor(() => expect(currency.disabled).toBe(false));
    expect(currency.value).toBe("EUR");
    expect(
      (
        within(dialog).getByRole("button", {
          name: "Speichern",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });

  it("says a refusal once — on the field, or in the summary, never both", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backendFor(SETTINGS_EDITOR, {
      refuse: [
        {
          field: "timezone",
          code: "too_long",
          message: "That zone is too long.",
          max: 64,
        },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<InstallationSettingsCard />);

    const dialog = await openFrom(user, /edit company time zone/i);
    const zone = within(dialog).getByLabelText(/company time zone/i);
    await user.type(zone, "/Extra");
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }));

    const refusal = await screen.findByText(
      "Company time zone: at most 64 characters.",
    );
    // One occurrence, the field's: the summary carries only what no field
    // claimed.
    expect(
      screen.getAllByText("Company time zone: at most 64 characters."),
    ).toHaveLength(1);
    await waitFor(() => expect(document.activeElement).toBe(zone));
    expect(refusal.className).toContain("field-error");
    expect(screen.getAllByRole("alert")).toHaveLength(1);
  });
});
