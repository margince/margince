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
