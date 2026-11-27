/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { ContractForm } from "./contractform";

// The unit an agreement is written in is one somebody stated: the reader's pick,
// the agreement's own, or the installation's declared base currency. Never a
// literal, which would label one deployment's agreements in another country's
// money and be indistinguishable from a currency an operator chose.
//
// The currency also decides the SCALE, so these cases pin both, on an
// installation whose currency is the dong, which has no minor unit.

const SETTINGS = {
  company_name: "Brandt Automotive GmbH",
  timezone: "Europe/Berlin",
  base_currency: "VND",
  base_currency_locked: false,
  max_upload_bytes: 26_214_400,
};

// An agreement on record with no money on it — the only shape the record's own
// CHECK allows a currency-less contract to have.
const CONTRACT: components["schemas"]["Contract"] = {
  id: "c-1",
  company_id: "o-1",
  title: "MSA 2026",
  status: "active",
  under_contract: true,
  term_state: "in_force",
  auto_renew: false,
  value_basis: "total",
  version: 1,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Every request the form makes, so the created agreement can be read off the
// wire rather than off a spy on our own helper.
function stubApi(settings: { body: unknown; status?: number }): Request[] {
  const seen: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      seen.push(request);
      const path = new URL(request.url).pathname;
      if (path.endsWith("/installation/settings")) {
        return jsonResponse(settings.body, settings.status);
      }
      if (path.includes("/contracts")) {
        return jsonResponse({ id: "c-1" });
      }
      return jsonResponse({ data: [] });
    }),
  );
  return seen;
}

function show(ui: ReactNode) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// Filling in the one agreement this suite records: a title, because the form
// refuses without one, and an amount, because an amount is what needs a unit.
async function recordAgreement(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/^Title/), "MSA 2026");
  await user.type(screen.getByLabelText("Value"), "5000");
  await user.click(
    screen.getByRole("button", { name: englishCopy["contracts.form.save"] }),
  );
}

// The value box, once the installation's currency has opened it.
async function openedValueBox() {
  await waitFor(() =>
    expect((screen.getByLabelText("Value") as HTMLInputElement).disabled).toBe(
      false,
    ),
  );
  return screen.getByLabelText("Value") as HTMLInputElement;
}

async function writtenBody(seen: Request[], method: "POST" | "PATCH") {
  const writes = seen.filter(
    (r) =>
      r.method === method && new URL(r.url).pathname.includes("/contracts"),
  );
  expect(writes.length).toBe(1);
  return await writes[0].clone().json();
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("recording an agreement's value", () => {
  it("writes the installation's own currency, not one this file chose", async () => {
    const user = userEvent.setup();
    const seen = stubApi({ body: SETTINGS });
    show(<ContractForm companyId="o-1" open onClose={() => {}} />);
    await openedValueBox();

    await recordAgreement(user);

    await waitFor(async () =>
      expect(await writtenBody(seen, "POST")).toMatchObject({
        // 5000 dong is 5000 minor units. VND has no minor unit at all, so the
        // scale is 1 — this file's whole premise is a dong installation, and
        // it used to assert the hundredfold figure the form produced.
        value_minor: 5_000,
        currency: "VND",
      }),
    );
  });

  it("writes the currency the reader picks, keeping the amount as typed", async () => {
    const user = userEvent.setup();
    const seen = stubApi({ body: SETTINGS });
    show(<ContractForm companyId="o-1" open onClose={() => {}} />);
    const value = await openedValueBox();
    await user.type(value, "5000");
    await user.tab();

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Currency" }),
      /^USD/,
    );
    await user.type(screen.getByLabelText(/^Title/), "MSA 2026");
    await user.click(screen.getByRole("button", { name: "Record contract" }));

    await waitFor(async () =>
      expect(await writtenBody(seen, "POST")).toMatchObject({
        // 5000 dollars, at the dollar's two decimals.
        value_minor: 500_000,
        currency: "USD",
      }),
    );
  });

  it("says the currency could not be read, and opens the amount once a retry reads it", async () => {
    const user = userEvent.setup();
    // The installation read is the only source of a unit here. Without it an
    // amount has no scale, so the box says why it is shut and offers the retry.
    let settingsUp = false;
    const seen: Request[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        seen.push(request);
        const path = new URL(request.url).pathname;
        if (path.endsWith("/installation/settings")) {
          return settingsUp
            ? jsonResponse(SETTINGS)
            : jsonResponse({ detail: "unavailable" }, 503);
        }
        if (path.includes("/contracts")) return jsonResponse({ id: "c-1" });
        return jsonResponse({ data: [] });
      }),
    );
    show(<ContractForm companyId="o-1" open onClose={() => {}} />);

    const [unread] = await screen.findAllByText(
      englishCopy["amount.currencyUnread"],
    );
    const value = screen.getByLabelText("Value") as HTMLInputElement;
    expect(value.disabled).toBe(true);
    expect(value.getAttribute("aria-describedby")).toContain(unread.id);
    expect(value.placeholder).toBe("");

    settingsUp = true;
    const retry = unread.querySelector("button");
    expect(retry?.textContent).toBe("Retry");
    if (retry) await user.click(retry);
    expect(await openedValueBox()).toHaveProperty("value", "");
    await recordAgreement(user);

    await waitFor(async () =>
      expect(await writtenBody(seen, "POST")).toMatchObject({
        value_minor: 5_000,
        currency: "VND",
      }),
    );
  });

  it("takes no amount while the base currency loads, then reads it at that currency's scale", async () => {
    const user = userEvent.setup();
    let answerSettings: (response: Response) => void = () => {};
    const settingsRead = new Promise<Response>((resolve) => {
      answerSettings = resolve;
    });
    const seen: Request[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        seen.push(request);
        const path = new URL(request.url).pathname;
        if (path.endsWith("/installation/settings")) return settingsRead;
        if (path.includes("/contracts")) return jsonResponse({ id: "c-1" });
        return jsonResponse({ data: [] });
      }),
    );
    show(<ContractForm companyId="o-1" open onClose={() => {}} />);
    await waitFor(() => expect(screen.getByLabelText(/^Title/)).toBeTruthy());
    const early = screen.getByLabelText("Value") as HTMLInputElement;
    expect(early.disabled).toBe(true);
    await user.type(early, "500");

    answerSettings(jsonResponse(SETTINGS));
    expect(await openedValueBox()).toHaveProperty("value", "");
    await recordAgreement(user);

    await waitFor(async () =>
      expect(await writtenBody(seen, "POST")).toMatchObject({
        value_minor: 5_000,
        currency: "VND",
      }),
    );
  });
});

describe("correcting an agreement's value", () => {
  it("keeps the currency the agreement already records", async () => {
    const user = userEvent.setup();
    const seen = stubApi({ body: SETTINGS });
    // A dollar agreement on an installation that reports in dong. Re-labelling
    // it would restate what was signed, so a correction to any other field
    // leaves the unit exactly as recorded.
    show(
      <ContractForm
        companyId="o-1"
        contract={{ ...CONTRACT, value_minor: 700_000, currency: "USD" }}
        open
        onClose={() => {}}
      />,
    );
    await waitFor(() => expect(screen.getByLabelText(/^Title/)).toBeTruthy());

    await user.click(
      screen.getByRole("button", {
        name: englishCopy["contracts.form.saveEdit"],
      }),
    );

    await waitFor(async () =>
      expect(await writtenBody(seen, "PATCH")).toMatchObject({
        value_minor: 700_000,
        currency: "USD",
      }),
    );
  });

  it("prices an agreement recorded without a value in the installation's currency", async () => {
    const user = userEvent.setup();
    const seen = stubApi({ body: SETTINGS });
    // The record's two money columns are paired, so an agreement carrying no
    // currency carries no amount either: it is being priced here for the first
    // time, which is the same act as pricing a new one.
    show(
      <ContractForm
        companyId="o-1"
        contract={CONTRACT}
        open
        onClose={() => {}}
      />,
    );
    await waitFor(() => expect(screen.getByLabelText(/^Title/)).toBeTruthy());

    await user.type(await openedValueBox(), "5000");
    await user.click(
      screen.getByRole("button", {
        name: englishCopy["contracts.form.saveEdit"],
      }),
    );

    await waitFor(async () =>
      expect(await writtenBody(seen, "PATCH")).toMatchObject({
        value_minor: 5_000,
        currency: "VND",
      }),
    );
  });
});
