/** @vitest-environment happy-dom */
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
import { en } from "../../i18n/en";
import { LocaleProvider } from "../../i18n/index";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { ConnectPostureStep } from "./connect-posture";
import { OAuthReturnPanel } from "./onboarding-connect-panels";

// The provider-agnostic return view: what a consent round trip came back
// with, read fresh from the roster rather than assumed from the route.

function render(ui: ReactNode) {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const googleRows = (calendar: boolean) => () =>
  jsonResponse({
    data: [
      {
        id: "g1",
        provider: "gmail",
        status: "connected",
        scopes: ["read"],
        backfill: { state: "done" },
      },
      ...(calendar
        ? [{ id: "c1", provider: "gcal", status: "connected", scopes: [] }]
        : []),
    ],
  });

// A mail-and-calendar press lands on the calendar's outcome, because the
// calendar is the last consent the server ran. The mailbox it chained after is
// the account panel's to show, so the landing speaks only of a failure.
describe("a chained calendar landing", () => {
  it("says nothing over a calendar that was granted", async () => {
    installFetchStub({ "GET /connectors": googleRows(true) });
    const { container } = render(
      <OAuthReturnPanel outcome="ok" provider="gcal" onDone={vi.fn()} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("names the refused calendar and never claims nothing connected", async () => {
    installFetchStub({ "GET /connectors": googleRows(false) });
    render(
      <OAuthReturnPanel outcome="denied" provider="gcal" onDone={vi.fn()} />,
    );
    expect(
      await screen.findByText(en["ob.conv.connect.calendarFailedTitle"]),
    ).toBeTruthy();
    expect(
      screen.getByText(en["ob.conv.connect.calendarDeclined"]),
    ).toBeTruthy();
    // "Nothing was connected" would be false: the mailbox is.
    expect(screen.queryByText(en["ob.s4.connectDenied"])).toBeNull();
  });

  it("claims no mailbox for a calendar asked for alone", async () => {
    installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
    render(
      <OAuthReturnPanel
        outcome="denied"
        provider="graphcal"
        onDone={vi.fn()}
      />,
    );
    expect(
      await screen.findByText(en["ob.conv.connect.calendarFailedTitle"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["ob.s4.connectConfirmFailed"])).toBeNull();
  });
});

it("OAuthReturnPanel says nothing once the roster confirms the mailbox", async () => {
  installFetchStub({
    "GET /connectors": () =>
      jsonResponse({
        data: [
          {
            id: "g1",
            provider: "graph",
            status: "connected",
            scopes: ["read"],
            backfill: { state: "done" },
          },
        ],
      }),
  });
  render(<OAuthReturnPanel outcome="ok" onDone={vi.fn()} />);
  await waitFor(() =>
    expect(screen.queryByText(en["ob.s4.connectVerifying"])).toBeNull(),
  );
  expect(screen.queryByText(en["ob.s4.connectConfirmFailed"])).toBeNull();
});

// A provider whose consent succeeded at the provider but whose row never
// landed is a confirm failure, not an invitation to import somebody else's
// mailbox.
it("OAuthReturnPanel reports a confirm-failure when the returning provider is not connected", async () => {
  installFetchStub({
    "GET /connectors": () =>
      jsonResponse({
        data: [
          { id: "g1", provider: "gmail", status: "connected", scopes: [] },
        ],
      }),
  });
  render(<OAuthReturnPanel outcome="ok" provider="graph" onDone={vi.fn()} />);
  expect(
    await screen.findByText("The connection could not be confirmed."),
  ).toBeTruthy();
});

// A segment this build cannot resolve to a provider is NOT the same as one that
// is absent. Absent is deploy skew and falls back to the roster; unresolved
// names no mailbox, and falling back there would offer to import an inbox the
// human never just connected — the very swap the exact match exists to prevent.
it("OAuthReturnPanel refuses to offer an import for an unrecognized provider segment", async () => {
  const statusReads: string[] = [];
  installFetchStub({
    "GET /connectors": () =>
      jsonResponse({
        data: [
          { id: "g1", provider: "gmail", status: "connected", scopes: [] },
        ],
      }),
    "GET /connectors/gmail/backfill": () => {
      statusReads.push("gmail");
      return jsonResponse({ state: "idle" });
    },
  });
  render(<OAuthReturnPanel outcome="ok" provider="bogus" onDone={vi.fn()} />);
  expect(
    await screen.findByText("The connection could not be confirmed."),
  ).toBeTruthy();
  expect(screen.queryByText("Live and capturing")).toBeNull();
  // The backfill panel reads the run for the mailbox it is offered for; the
  // connected Gmail row must never be that mailbox here.
  expect(statusReads).toEqual([]);
});

it("OAuthReturnPanel reports a confirm-failure when no connection came back", async () => {
  installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
  render(<OAuthReturnPanel outcome="ok" onDone={vi.fn()} />);
  expect(
    await screen.findByText("The connection could not be confirmed."),
  ).toBeTruthy();
});

// Onboarding is the DEFAULT return surface for a consent round-trip, so it sees
// the same server outcome enum Settings does. A permanent failure that only
// Settings knows about would fall through to this panel's generic advice —
// "try connecting again" — which is the one thing that cannot work here.
it("OAuthReturnPanel names the remedy when the provider's API is not enabled", async () => {
  installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
  render(<OAuthReturnPanel outcome="misconfigured" onDone={vi.fn()} />);
  expect(
    await screen.findByText(en["connectors.oauthMisconfigured"]),
  ).toBeTruthy();
  expect(screen.queryByText(en["ob.s4.connectRetry"])).toBeNull();
});

it("OAuthReturnPanel tells the reader what to accept when the provider declined", async () => {
  installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
  render(<OAuthReturnPanel outcome="rejected" onDone={vi.fn()} />);
  expect(await screen.findByText(en["connectors.oauthRejected"])).toBeTruthy();
  // Retrying IS the right advice once the permissions are accepted, so this copy
  // may say so — what it must not do is fall back to the generic panel text that
  // names no remedy at all.
  expect(screen.queryByText(en["ob.s4.connectRetry"])).toBeNull();
});

// An outcome this panel does not know must still land on the honest generic
// failure rather than rendering nothing.
it("OAuthReturnPanel keeps the generic failure for an unrecognized outcome", async () => {
  installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
  render(<OAuthReturnPanel outcome="something-new" onDone={vi.fn()} />);
  expect(
    await screen.findByText(en["ob.s4.connectConfirmFailed"]),
  ).toBeTruthy();
});

// The posture question rides the connect flow because the import reads a year
// of mail. An answer given on a later screen arrives after every message it
// was meant to govern. It is saved on the choice.
describe("the posture step", () => {
  function renderPosture(posture: string | undefined, sharedAllowed = false) {
    const bodies: { posture: string; apply: boolean }[] = [];
    installFetchStub({
      "GET /capture/settings": () =>
        jsonResponse({ shared_posture_allowed: sharedAllowed }),
      // A non-GET handler is handed the PARSED body, never a Request.
      "PUT /connectors/gmail/mail-posture": (body: unknown) => {
        const b = body as { posture: string; apply_to_history: boolean };
        bodies.push({ posture: b.posture, apply: b.apply_to_history });
        return jsonResponse({});
      },
    });
    render(
      <ConnectPostureStep
        provider="gmail"
        posture={posture === undefined ? undefined : (posture as "held")}
      />,
    );
    return bodies;
  }

  it("offers the three postures, refusing shared until an admin allows it", async () => {
    renderPosture(undefined);
    // The refused option is PRESENT and disabled, never absent: a missing
    // third answer tells a reader their product has two postures.
    const shared = await screen.findByRole("radio", {
      name: new RegExp(en["connectors.mailPosture.shared"]),
    });
    await waitFor(() => expect(shared).toBeDisabled());
    expect(
      screen.getByText(en["connectors.mailPosture.sharedNeedsAdmin"], {
        exact: false,
      }),
    ).toBeTruthy();
  });

  // A connection is born `classified` from the column default, so the control
  // reports what the server already wrote rather than a local guess.
  it("shows the posture the server stored, not a client-side default", async () => {
    renderPosture("held", true);
    expect(
      await screen.findByRole("radio", {
        name: new RegExp(en["connectors.mailPosture.held"]),
      }),
    ).toBeChecked();
  });

  // A same-account reconnect lands on the row it already had, so this screen
  // cannot assume nothing is captured yet. Narrowing carries the history with
  // it, and widening never opens it as a side effect.
  it("saves on the choice, narrowing the mail already captured", async () => {
    const bodies = renderPosture("classified");
    await userEvent.click(
      await screen.findByRole("radio", {
        name: new RegExp(en["connectors.mailPosture.held"]),
      }),
    );
    await waitFor(() =>
      expect(bodies).toEqual([{ posture: "held", apply: true }]),
    );
  });

  it("widens without touching the mail already captured", async () => {
    const bodies = renderPosture("held");
    await userEvent.click(
      await screen.findByRole("radio", {
        name: new RegExp(en["connectors.mailPosture.classified"]),
      }),
    );
    await waitFor(() =>
      expect(bodies).toEqual([{ posture: "classified", apply: false }]),
    );
  });
});
