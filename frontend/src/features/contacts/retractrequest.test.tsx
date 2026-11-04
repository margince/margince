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
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { installFetchStub } from "../../testing/story-utils";
import {
  RetractAcquisitionRequest,
  RetractRecordedRequest,
} from "./retractrequest";

// Taking back a request a colleague recorded by hand. The button shows only
// where the guard names the exchange by id. It asks once, then retracts.

type QualifyingEvent = components["schemas"]["ConsentQualifyingEvent"];

const RECORDED: QualifyingEvent = {
  id: "0193a5c0-0000-7000-8000-00000000e1e1",
  kind: "requested_by_subject",
  occurred_at: "2026-10-02T09:00:00Z",
  note: "Asked on the phone for our price list.",
};

const RETRACT_ROUTE = `POST /contacts/p-1/consent/qualifying-events/${RECORDED.id}/retract`;

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

describe("taking back a recorded request", () => {
  it("asks first, then retracts the exchange the guard named", async () => {
    const user = userEvent.setup();
    const retracted = vi.fn(() => new Response(null, { status: 204 }));
    installFetchStub({ [RETRACT_ROUTE]: retracted });
    render(
      <RetractRecordedRequest contactId="p-1" event={RECORDED} mayWrite />,
    );

    await user.click(
      screen.getByRole("button", { name: "Take back this request" }),
    );
    expect(retracted).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/cannot be restored/, { exact: false }),
    ).toBeTruthy();

    const dialog = screen.getByRole("dialog");
    const confirm = Array.from(dialog.querySelectorAll("button")).find(
      (button) => button.textContent === "Take back this request",
    );
    if (!confirm) throw new Error("the dialog has no confirm button");
    await user.click(confirm);

    await waitFor(() => expect(retracted).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  // A derived exchange, such as an inbound mail, carries no id. A reader who
  // may not edit the contact gets no button either.
  it("offers nothing for a derived exchange or a reader who may not edit", () => {
    const derived: QualifyingEvent = {
      kind: "inbound_message",
      occurred_at: RECORDED.occurred_at,
    };
    render(
      <>
        <RetractRecordedRequest contactId="p-1" event={derived} mayWrite />
        <RetractRecordedRequest
          contactId="p-1"
          event={RECORDED}
          mayWrite={false}
        />
      </>,
    );
    expect(screen.queryByRole("button", { name: /Take back/ })).toBeNull();
  });
});

describe("taking back the request the acquisition records", () => {
  it("asks first, then retracts it", async () => {
    const user = userEvent.setup();
    const retracted = vi.fn(() => new Response(null, { status: 204 }));
    installFetchStub({
      "POST /contacts/p-1/acquisition-request/retract": retracted,
    });
    render(
      <RetractAcquisitionRequest
        contactId="p-1"
        request={{ kinds: ["requested_quote_or_meeting"] }}
        mayWrite
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Take back this request" }),
    );
    expect(retracted).not.toHaveBeenCalled();
    const dialog = await screen.findByRole("dialog");
    const confirm = Array.from(dialog.querySelectorAll("button")).find(
      (button) => button.textContent === "Take back this request",
    );
    if (!confirm) throw new Error("the dialog has no confirm button");
    await user.click(confirm);

    await waitFor(() => expect(retracted).toHaveBeenCalledTimes(1));
  });

  it("offers nothing when no request stands or the reader may not edit", () => {
    render(
      <>
        <RetractAcquisitionRequest
          contactId="p-1"
          request={undefined}
          mayWrite
        />
        <RetractAcquisitionRequest
          contactId="p-1"
          request={{ kinds: ["in_person_permission"] }}
          mayWrite={false}
        />
      </>,
    );
    expect(screen.queryByRole("button", { name: /Take back/ })).toBeNull();
  });
});
