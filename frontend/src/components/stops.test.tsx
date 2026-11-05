/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { ContactStops, LeadStops } from "./stops";

// The stops on a contact. The server says per stop whether this reader may take
// it back; the panel offers that, with a required reason.

type ContactStop = components["schemas"]["ContactStop"];

const STOPS_ROUTE = "GET /contacts/p-1/consent/stops";

const TYPED_OBJECTION: ContactStop = {
  id: "0193a5c0-0000-7000-8000-00000000a001",
  kind: "marketing_objection",
  authority: "subject",
  reason: "said no newsletters",
  recorded_at: "2026-10-02T09:00:00Z",
  recorded_by_colleague: true,
  recorded_by_name: "Rita Rep",
  may_lift: true,
  correction: true,
};

const PRESSED_OBJECTION: ContactStop = {
  ...TYPED_OBJECTION,
  id: "0193a5c0-0000-7000-8000-00000000a002",
  reason: null,
  recorded_by_colleague: false,
  recorded_by_name: null,
  may_lift: false,
  correction: false,
  lift_blocked_by: "subject_decision",
};

const RESTRICTION: ContactStop = {
  ...PRESSED_OBJECTION,
  id: "0193a5c0-0000-7000-8000-00000000a003",
  kind: "processing_restriction",
  lift_blocked_by: "restriction",
};

function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the stops on a contact", () => {
  it("corrects a stop a colleague typed, only once a reason is given", async () => {
    const user = userEvent.setup();
    const lifted = vi.fn(
      (_body: unknown) => new Response(null, { status: 204 }),
    );
    installFetchStub({
      [STOPS_ROUTE]: () => jsonResponse({ stops: [TYPED_OBJECTION] }),
      [`POST /contacts/p-1/consent/suppress/${TYPED_OBJECTION.id}/lift`]:
        lifted,
    });
    render(<ContactStops contactId="p-1" />);

    expect(await screen.findByText("Objected to marketing")).toBeTruthy();
    expect(screen.getByText(/Recorded by Rita Rep/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Correct this stop" }));

    const dialog = await screen.findByRole("dialog");
    const confirm = Array.from(dialog.querySelectorAll("button")).find(
      (button) => button.textContent === "Correct this stop",
    );
    if (!confirm) throw new Error("the dialog has no confirm button");
    expect(confirm.disabled).toBe(true);

    await user.type(screen.getByRole("textbox"), "typed on the wrong contact");
    expect(confirm.disabled).toBe(false);
    await user.click(confirm);

    await waitFor(() => expect(lifted).toHaveBeenCalledTimes(1));
    expect(lifted.mock.calls[0]?.[0]).toEqual({
      reason: "typed on the wrong contact",
    });
  });

  it("says why the contact's own stop and a restriction stay", async () => {
    installFetchStub({
      [STOPS_ROUTE]: () =>
        jsonResponse({ stops: [PRESSED_OBJECTION, RESTRICTION] }),
    });
    render(<ContactStops contactId="p-1" />);

    expect(
      await screen.findByText(
        "The contact set this stop. Only they can change it.",
      ),
    ).toBeTruthy();
    expect(screen.getByText(/means telling the contact first/)).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("the stops on a lead", () => {
  it("lists the lead's stops and lifts one through the lead's route", async () => {
    const user = userEvent.setup();
    const lifted = vi.fn(
      (_body: unknown) => new Response(null, { status: 204 }),
    );
    installFetchStub({
      "GET /leads/l-1/consent/stops": () =>
        jsonResponse({ stops: [TYPED_OBJECTION] }),
      [`POST /leads/l-1/consent/suppress/${TYPED_OBJECTION.id}/lift`]: lifted,
    });
    render(<LeadStops leadId="l-1" />);

    expect(await screen.findByText("Objected to marketing")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Correct this stop" }));
    await user.type(
      await screen.findByRole("textbox"),
      "typed on the wrong lead",
    );
    const confirm = Array.from(
      (await screen.findByRole("dialog")).querySelectorAll("button"),
    ).find((button) => button.textContent === "Correct this stop");
    if (!confirm) throw new Error("the dialog has no confirm button");
    await user.click(confirm);

    await waitFor(() => expect(lifted).toHaveBeenCalledTimes(1));
    expect(lifted.mock.calls[0]?.[0]).toEqual({
      reason: "typed on the wrong lead",
    });
  });
});
