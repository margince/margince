/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
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
import { ComposeModal } from "./compose";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
  refusedPreview,
} from "./sendpermission.testkit";

// The composer asks the engine whether the message may go, and says so where
// the rep is writing it.
//
// Until now a rep learned a send was refused by pressing Send and reading an
// error. The preview endpoint has answered since the engine landed; these hold
// that the composer asks it, asks the RIGHT question, and draws the answer in
// the shared component's words rather than its own.

type Activity = components["schemas"]["Activity"];
type Preview = components["schemas"]["SendAuthorizationPreview"];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const inbound: Activity = {
  id: "act-1",
  kind: "email",
  direction: "inbound",
  occurred_at: "2026-08-01T09:00:00Z",
  is_done: false,
  source: "capture",
  captured_by: "system:capture",
  subject: "The quote",
  created_at: "2026-08-01T09:00:00Z",
  updated_at: "2026-08-01T09:00:00Z",
};

type Ask = { path: string; body: unknown };

/**
 * Every route the composer reads answers empty; the anchor answers with the
 * inbound message; the preview doors answer however the case says.
 */
function stubEngine(engine: (addresses: string[]) => Preview | Response) {
  const asks: Ask[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const path = url.pathname.replace(/^\/v1/, "");
      if (path === "/activities/act-1") return jsonResponse(inbound);
      if (isPreviewDoor(path)) {
        let body: unknown = null;
        try {
          body = request
            ? await request.clone().json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
        asks.push({ path, body });
        const answer = engine(previewedAddresses(body));
        return answer instanceof Response ? answer : jsonResponse(answer);
      }
      return jsonResponse({ data: [] });
    }),
  );
  return asks;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function openReplyAndAddress(address: string) {
  render(
    <ComposeModal
      activityId="act-1"
      entityType="person"
      entityId="p-1"
      open
      onClose={vi.fn()}
    />,
  );
  await screen.findByText(
    "This continues their own message, so it needs no reason from you.",
  );
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("To"), address);
  await user.tab();
}

describe("the composer asks before anybody presses Send", () => {
  it("says who decided against the message, and that nobody may lift it", async () => {
    const asks = stubEngine((addresses) =>
      addresses.length === 0
        ? allowedPreview([])
        : refusedPreview(addresses[0] ?? "", {
            reason_code: "marketing_objection",
            decided_by: "subject",
          }),
    );
    await openReplyAndAddress("anna@example.test");

    expect(
      await screen.findByText(/asked not to receive marketing/i),
    ).toBeInTheDocument();
    // The question is about THIS message: the thread being answered and the
    // addressee the rep named, through the reply door the send will use.
    const asked = asks.find((ask) => previewedAddresses(ask.body).length > 0);
    expect(asked?.path).toBe("/activities/act-1/send-email:preview");
    expect(previewedAddresses(asked?.body)).toEqual(["anna@example.test"]);
  });

  // The overwhelming majority of sends. It has to cost no attention at all.
  it("says nothing when the engine allows the message", async () => {
    const asks = stubEngine(allowedPreview);
    await openReplyAndAddress("anna@example.test");

    // The absence means something only once the question about the addressee
    // has gone out; before that, nothing is drawn for the uninteresting reason
    // that nothing has been asked.
    await waitFor(() =>
      expect(asks.some((ask) => previewedAddresses(ask.body).length > 0)).toBe(
        true,
      ),
    );
    expect(screen.queryByText(/cannot send this message/i)).toBeNull();
    expect(screen.queryByText(/no record of why/i)).toBeNull();
    expect(screen.queryByText(/could not check/i)).toBeNull();
  });

  // No override exists yet on the server, so no control is drawn: a button here
  // would promise a lift the staging gate refuses. The state still explains
  // itself, and says what happens to the message as it stands.
  it("explains an unproven send without offering a control", async () => {
    stubEngine((addresses) =>
      addresses.length === 0
        ? allowedPreview([])
        : refusedPreview(addresses[0] ?? "", {
            reason_code: "no_compatible_evidence",
            decided_by: "machine",
            can_be_overruled: true,
          }),
    );
    await openReplyAndAddress("stranger@example.test");

    expect(
      await screen.findByText(/no record of why you may write/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/will be refused/i)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /say why you may write/i }),
    ).toBeNull();
  });

  // A 200 that is not a preview — a proxy's page, a server of another version
  // — is not an answer either, and must not take the composer down with it:
  // the message is still the rep's to send, and the door still refuses it if
  // it must.
  it("says the check did not happen on an answer it cannot read", async () => {
    stubEngine((addresses) =>
      addresses.length === 0 ? allowedPreview([]) : jsonResponse({}),
    );
    await openReplyAndAddress("anna@example.test");

    expect(await screen.findByText(/could not check/i)).toBeInTheDocument();
    expect(screen.getByLabelText("To")).toBeInTheDocument();
  });

  // A failed check is not permission. An empty 502 from a gateway carries no
  // body, and a composer that read "no error" as "allowed" would fall silent on
  // exactly the message it could not check.
  it("says the check did not happen on a bodiless failure", async () => {
    stubEngine((addresses) =>
      addresses.length === 0
        ? allowedPreview([])
        : new Response(null, { status: 502 }),
    );
    await openReplyAndAddress("anna@example.test");

    expect(await screen.findByText(/could not check/i)).toBeInTheDocument();
  });
});
