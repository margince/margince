/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider, translate } from "../i18n";
import { LawfulBasisCallout, useLawfulBasisGate } from "./lawfulbasiswarning";

type Preview = components["schemas"]["SendAuthorizationPreview"];

// A recipient the server marks as having no lawful basis on record, beside one
// it does not mark.
const MARKED: Preview = {
  allowed: true,
  recipients: [
    {
      address: "lan@example.test",
      verdict: "allow",
      reason_code: "allowed",
      lawful_basis_missing: true,
      consent_purpose_id: "018f3a1b-0000-7000-8000-0000000000c0",
      evidence_subject: {
        entity_type: "contact",
        entity_id: "018f3a1b-0000-7000-8000-000000000042",
      },
    },
    { address: "anna@example.test", verdict: "allow", reason_code: "allowed" },
  ],
};

function Sender({
  preview,
  onSend,
}: Readonly<{ preview: Preview; onSend: () => void }>) {
  const gate = useLawfulBasisGate(preview);
  return (
    <>
      <LawfulBasisCallout preview={preview} />
      <button type="button" onClick={() => gate.gate(onSend)}>
        Send
      </button>
      {gate.dialog}
    </>
  );
}

function draw(preview: Preview, onSend: () => void) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Sender preview={preview} onSend={onSend} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the no-lawful-basis warning", () => {
  it("warns, and sends only after the confirmation is recorded", async () => {
    const fetched = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ confirmed: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const onSend = vi.fn();
    draw(MARKED, onSend);

    expect(
      screen.getAllByText(translate("en", "lawfulBasis.title")).length,
    ).toBeGreaterThan(0);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(onSend).not.toHaveBeenCalled();
    await userEvent.click(
      screen.getByRole("button", {
        name: translate("en", "lawfulBasis.confirm"),
      }),
    );

    await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
    const request = fetched.mock.calls[0]?.[0];
    expect(request).toBeInstanceOf(Request);
    if (!(request instanceof Request)) return;
    expect(request.url).toContain("/lawful-basis-confirmations");
    expect(await request.json()).toEqual({
      subjects: [
        {
          entity_type: "contact",
          entity_id: "018f3a1b-0000-7000-8000-000000000042",
        },
      ],
    });
  });

  it("states that no valid basis is recorded and offers both ways on", async () => {
    vi.spyOn(globalThis, "fetch");
    draw(MARKED, vi.fn());
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(
      screen.getAllByText("No valid basis recorded").length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText(
        "No valid basis under Vietnamese law is recorded for this contact: neither their consent nor a case where the law allows processing without it. Sending is at your own risk.",
      ).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: "Ask for consent" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Send anyway" }),
    ).toBeInTheDocument();
  });

  it("asks the contact for consent and does not send the message", async () => {
    const fetched = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          delivered_to: "lan@example.test",
          expires_at: "2026-11-24T00:00:00Z",
          queued: true,
          sendable: true,
        }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
    );
    const onSend = vi.fn();
    draw(MARKED, onSend);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    await userEvent.click(
      screen.getByRole("button", {
        name: translate("en", "lawfulBasis.askConsent"),
      }),
    );

    await waitFor(() => expect(fetched).toHaveBeenCalledTimes(1));
    const request = fetched.mock.calls[0]?.[0];
    expect(request).toBeInstanceOf(Request);
    if (!(request instanceof Request)) return;
    expect(request.url).toContain(
      "/contacts/018f3a1b-0000-7000-8000-000000000042/consent/double-opt-in",
    );
    expect(await request.json()).toEqual({
      purpose_id: "018f3a1b-0000-7000-8000-0000000000c0",
    });
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Send anyway" }),
      ).not.toBeInTheDocument(),
    );
    expect(onSend).not.toHaveBeenCalled();
  });

  it("sends at once when no recipient is marked", async () => {
    const fetched = vi.spyOn(globalThis, "fetch");
    const onSend = vi.fn();
    const clean: Preview = {
      allowed: true,
      recipients: [MARKED.recipients[1]].filter((r) => r !== undefined),
    };
    draw(clean, onSend);

    expect(
      screen.queryByText(translate("en", "lawfulBasis.title")),
    ).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(onSend).toHaveBeenCalledTimes(1);
    expect(fetched).not.toHaveBeenCalled();
  });
});
