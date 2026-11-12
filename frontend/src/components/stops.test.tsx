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
import { ContactRestrictedMark, ContactStops, LeadStops } from "./stops";

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

describe("lifting a restriction", () => {
  it("lets an admin lift it, which queues the notice", async () => {
    const user = userEvent.setup();
    const asked = vi.fn(
      (_body: unknown) => new Response(null, { status: 202 }),
    );
    installFetchStub({
      [STOPS_ROUTE]: () =>
        jsonResponse({
          stops: [{ ...RESTRICTION, may_lift_restriction: true }],
        }),
      [`POST /contacts/p-1/consent/suppress/${RESTRICTION.id}/lift-restriction`]:
        asked,
    });
    render(<ContactStops contactId="p-1" />);

    await user.click(
      await screen.findByRole("button", { name: "Lift the restriction" }),
    );
    expect(
      await screen.findByText(/first sends the contact a notice/),
    ).toBeTruthy();
    await user.type(screen.getByRole("textbox"), "the dispute is settled");
    const confirm = Array.from(
      (await screen.findByRole("dialog")).querySelectorAll("button"),
    ).find((button) => button.textContent === "Lift the restriction");
    if (!confirm) throw new Error("the dialog has no confirm button");
    await user.click(confirm);

    await waitFor(() => expect(asked).toHaveBeenCalledTimes(1));
    expect(asked.mock.calls[0]?.[0]).toEqual({
      reason: "the dispute is settled",
    });
  });

  it("says the restriction stays while the notice waits or after it failed", async () => {
    installFetchStub({
      [STOPS_ROUTE]: () =>
        jsonResponse({
          stops: [
            {
              ...RESTRICTION,
              lift_notice: {
                status: "queued",
                queued_at: "2026-11-12T09:00:00Z",
              },
            },
            {
              ...RESTRICTION,
              id: "0193a5c0-0000-7000-8000-00000000a004",
              lift_notice: {
                status: "failed",
                reason: "the mailbox refused the address",
                queued_at: "2026-11-12T09:00:00Z",
              },
            },
          ],
        }),
    });
    render(<ContactStops contactId="p-1" />);

    expect(
      await screen.findByText(/waiting to be sent\. The restriction stays/),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "The notice did not go out, so the restriction stays: the mailbox refused the address",
      ),
    ).toBeTruthy();
  });
});

describe("a lift already underway", () => {
  it("offers no second lift while the notice waits or after it was sent", async () => {
    installFetchStub({
      [STOPS_ROUTE]: () =>
        jsonResponse({
          stops: [
            {
              ...RESTRICTION,
              may_lift_restriction: true,
              lift_notice: {
                status: "queued",
                queued_at: "2026-11-12T09:00:00Z",
              },
            },
            {
              ...RESTRICTION,
              id: "0193a5c0-0000-7000-8000-00000000a005",
              may_lift_restriction: true,
              lift_notice: {
                status: "sent",
                queued_at: "2026-11-12T09:00:00Z",
              },
            },
          ],
        }),
    });
    render(<ContactStops contactId="p-1" />);

    expect(
      await screen.findByText(
        "The notice was sent. The restriction is ending.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Lift the restriction" }),
    ).toBeNull();
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

describe("the restriction mark on a contact", () => {
  it("shows while the contact holds a restriction", async () => {
    installFetchStub({
      [STOPS_ROUTE]: () =>
        jsonResponse({ stops: [TYPED_OBJECTION, RESTRICTION] }),
    });
    render(<ContactRestrictedMark contactId="p-1" />);
    expect(await screen.findByText("Processing is restricted")).toBeTruthy();
  });

  it("stays away for a stop that is not a restriction", async () => {
    const stops = vi.fn(() => jsonResponse({ stops: [TYPED_OBJECTION] }));
    installFetchStub({ [STOPS_ROUTE]: stops });
    render(<ContactRestrictedMark contactId="p-1" />);
    await waitFor(() => expect(stops).toHaveBeenCalled());
    expect(screen.queryByText("Processing is restricted")).toBeNull();
  });
});
