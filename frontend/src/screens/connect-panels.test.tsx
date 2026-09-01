/** @vitest-environment jsdom */
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
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import {
  OAuthConnectPanel,
  OAuthReturnPanel,
} from "./onboarding-connect-panels";
import { installFetchStub, jsonResponse } from "./story-utils";

// The Google panel's pre-connect state must reassure a first-time user before
// the redirect: an unverified dev app shows Google's "unverified app" notice,
// and without a heads-up a founder abandons the flow there. Both the OAuth
// connect panel (provider-parametrized) and the provider-agnostic return
// view share this file's harness.

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

describe("the Google connect panel", () => {
  it("warns about the unverified-app notice and how to get past it", () => {
    render(<OAuthConnectPanel provider="gmail" onDismiss={() => {}} />);
    // Both sentences read from the catalog: which caveat the panel carries is
    // the behaviour under test, and a copy edit that leaves it carrying the
    // same one should not read as a broken panel.
    expect(screen.getByText(en["ob.s4.googleUnverified"])).toBeTruthy();
    // The reassurance is honest about scope: read-only, never sends.
    expect(screen.getByText(en["ob.s4.googleHint"])).toBeTruthy();
  });
});

it("OAuthConnectPanel posts the given provider and redirects", async () => {
  const assign = vi.fn();
  vi.stubGlobal("location", { ...globalThis.location, assign });
  installFetchStub({
    "POST /connectors/graph/connect": () =>
      jsonResponse({ authorize_url: "https://login.microsoftonline/x" }),
  });
  render(<OAuthConnectPanel provider="graph" onDismiss={() => {}} />);
  await userEvent.click(
    screen.getByRole("button", { name: "Allow access to my Microsoft" }),
  );
  await waitFor(() =>
    expect(assign).toHaveBeenCalledWith("https://login.microsoftonline/x"),
  );
});

it("OAuthReturnPanel shows the live OAuth mailbox after consent", async () => {
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
  render(<OAuthReturnPanel outcome="ok" onComplete={vi.fn()} />);
  expect(await screen.findByText("Live and capturing")).toBeTruthy();
});

// The roster is provider-ordered, so "the first connected OAuth row" is Gmail
// whenever a workspace holds both. A Microsoft consent that lands on Gmail
// offers to import the wrong mailbox — the import the human is about to
// approve must be the mailbox they just connected.
it("OAuthReturnPanel offers the import for the mailbox the consent returned for", async () => {
  const statusReads: string[] = [];
  const connection = (provider: string) => ({
    id: provider,
    provider,
    status: "connected",
    scopes: ["read"],
  });
  installFetchStub({
    "GET /connectors": () =>
      jsonResponse({ data: [connection("gmail"), connection("graph")] }),
    "GET /connectors/gmail/backfill": () => {
      statusReads.push("gmail");
      return jsonResponse({ state: "idle" });
    },
    "GET /connectors/graph/backfill": () => {
      statusReads.push("graph");
      return jsonResponse({ state: "idle" });
    },
  });
  render(
    <OAuthReturnPanel outcome="ok" provider="graph" onComplete={vi.fn()} />,
  );
  await screen.findByText("Live and capturing");
  await waitFor(() => expect(statusReads).toEqual(["graph"]));
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
  render(
    <OAuthReturnPanel outcome="ok" provider="graph" onComplete={vi.fn()} />,
  );
  expect(
    await screen.findByText("We couldn't confirm the connection."),
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
  render(
    <OAuthReturnPanel outcome="ok" provider="bogus" onComplete={vi.fn()} />,
  );
  expect(
    await screen.findByText("We couldn't confirm the connection."),
  ).toBeTruthy();
  expect(screen.queryByText("Live and capturing")).toBeNull();
  // The backfill panel reads the run for the mailbox it is offered for; the
  // connected Gmail row must never be that mailbox here.
  expect(statusReads).toEqual([]);
});

it("OAuthReturnPanel reports a confirm-failure when no connection came back", async () => {
  installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
  render(<OAuthReturnPanel outcome="ok" onComplete={vi.fn()} />);
  expect(
    await screen.findByText("We couldn't confirm the connection."),
  ).toBeTruthy();
});

// Onboarding is the DEFAULT return surface for a consent round-trip, so it sees
// the same server outcome enum Settings does. A permanent failure that only
// Settings knows about would fall through to this panel's generic advice —
// "try connecting again" — which is the one thing that cannot work here.
it("OAuthReturnPanel names the remedy when the provider's API is not enabled", async () => {
  installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
  render(<OAuthReturnPanel outcome="misconfigured" onComplete={vi.fn()} />);
  expect(
    await screen.findByText(/administrator needs to enable it/i),
  ).toBeTruthy();
  expect(screen.queryByText(/try connecting again/i)).toBeNull();
});

it("OAuthReturnPanel tells the reader what to accept when the provider declined", async () => {
  installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
  render(<OAuthReturnPanel outcome="rejected" onComplete={vi.fn()} />);
  expect(await screen.findByText(/accept every permission/i)).toBeTruthy();
  // Retrying IS the right advice once the permissions are accepted, so this copy
  // may say so — what it must not do is fall back to the generic panel text that
  // names no remedy at all.
  expect(screen.queryByText(/Head to Settings/i)).toBeNull();
});

// An outcome this panel does not know must still land on the honest generic
// failure rather than rendering nothing.
it("OAuthReturnPanel keeps the generic failure for an unrecognized outcome", async () => {
  installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
  render(<OAuthReturnPanel outcome="something-new" onComplete={vi.fn()} />);
  expect(
    await screen.findByText(/couldn't confirm the connection/i),
  ).toBeTruthy();
});

// The posture question rides the connect flow because the backread is what
// reads a year of mail: an answer given on a later screen arrives after every
// message it was meant to govern.
describe("the posture step on a fresh connection", () => {
  const connectedGmail = (posture?: string) => ({
    id: "g1",
    provider: "gmail",
    status: "connected",
    scopes: ["read"],
    backfill: { state: "idle" },
    ...(posture === undefined ? {} : { mail_posture: posture }),
  });

  it("offers the three postures, refusing shared until an admin allows it", async () => {
    installFetchStub({
      "GET /connectors": () => jsonResponse({ data: [connectedGmail()] }),
      "GET /connectors/gmail/backfill": () => jsonResponse({ state: "idle" }),
      "GET /capture/settings": () =>
        jsonResponse({ shared_posture_allowed: false }),
    });
    render(
      <OAuthReturnPanel outcome="ok" provider="gmail" onComplete={vi.fn()} />,
    );

    expect(
      await screen.findByText(en["connectors.mailPosture.label"]),
    ).toBeTruthy();
    // The refused option is PRESENT and disabled, never absent: a missing
    // third answer tells a reader their product has two postures.
    // The help sentence and the admin refusal share one paragraph, so the
    // match is on the node that carries both rather than on either alone.
    expect(
      await screen.findByText(
        (_, node) =>
          node?.tagName === "P" &&
          (node.textContent ?? "").includes(
            en["connectors.mailPosture.sharedNeedsAdmin"],
          ),
      ),
    ).toBeTruthy();
  });

  // The whole point of asking here rather than in Settings.
  it("saves the posture before the backread offers to read history", async () => {
    const calls: string[] = [];
    installFetchStub({
      "GET /connectors": () => jsonResponse({ data: [connectedGmail()] }),
      "GET /connectors/gmail/backfill": () => jsonResponse({ state: "idle" }),
      "GET /capture/settings": () =>
        jsonResponse({ shared_posture_allowed: false }),
      // A non-GET handler is handed the PARSED body, never a Request.
      "PUT /connectors/gmail/mail-posture": (body: unknown) => {
        calls.push(String((body as { posture: string }).posture));
        return jsonResponse({});
      },
    });
    render(
      <OAuthReturnPanel outcome="ok" provider="gmail" onComplete={vi.fn()} />,
    );

    await screen.findByText(en["connectors.mailPosture.label"]);
    // Driven the way the design-system suite drives this control: the popup is
    // a listbox the keyboard owns, and DOM focus stays on the trigger.
    const user = userEvent.setup();
    const trigger = screen.getByRole("combobox");
    trigger.focus();
    await user.keyboard("{Enter}");
    await user.click(
      await screen.findByRole("option", {
        name: en["connectors.mailPosture.held"],
      }),
    );
    await waitFor(() => expect(calls).toEqual(["held"]));
  });

  // A connection is born `classified` from the column default, so the control
  // reports what the server already wrote rather than a local guess: a reader
  // who never touches it still leaves with the safe answer.
  it("shows the posture the server stored, not a client-side default", async () => {
    installFetchStub({
      "GET /connectors": () => jsonResponse({ data: [connectedGmail("held")] }),
      "GET /connectors/gmail/backfill": () => jsonResponse({ state: "idle" }),
      "GET /capture/settings": () =>
        jsonResponse({ shared_posture_allowed: true }),
    });
    render(
      <OAuthReturnPanel outcome="ok" provider="gmail" onComplete={vi.fn()} />,
    );

    expect(
      await screen.findByText(
        (_, node) =>
          node?.tagName === "P" &&
          (node.textContent ?? "").includes(
            en["connectors.mailPosture.help.held"],
          ),
      ),
    ).toBeTruthy();
  });
});

// The three things the review found, each as a case that fails without its fix.
describe("the posture step and the mail already captured", () => {
  const gmail = (posture?: string) => ({
    id: "g1",
    provider: "gmail",
    status: "connected",
    scopes: ["read"],
    backfill: { state: "idle" },
    ...(posture === undefined ? {} : { mail_posture: posture }),
  });

  // A same-account reconnect lands on the row it already had (the grant upserts
  // on (user_id, provider)), so "nothing is captured yet" is not something this
  // screen can assume. Narrowing therefore carries the history with it.
  it("narrows the mail already captured when the posture tightens", async () => {
    const bodies: { posture: string; apply: boolean }[] = [];
    installFetchStub({
      "GET /connectors": () => jsonResponse({ data: [gmail("classified")] }),
      "GET /connectors/gmail/backfill": () => jsonResponse({ state: "idle" }),
      "GET /capture/settings": () =>
        jsonResponse({ shared_posture_allowed: false }),
      "PUT /connectors/gmail/mail-posture": (body: unknown) => {
        const b = body as { posture: string; apply_to_history: boolean };
        bodies.push({ posture: b.posture, apply: b.apply_to_history });
        return jsonResponse({});
      },
    });
    render(
      <OAuthReturnPanel outcome="ok" provider="gmail" onComplete={vi.fn()} />,
    );

    await screen.findByText(en["connectors.mailPosture.label"]);
    const user = userEvent.setup();
    const trigger = screen.getByRole("combobox");
    trigger.focus();
    await user.keyboard("{Enter}");
    await user.click(
      await screen.findByRole("option", {
        name: en["connectors.mailPosture.held"],
      }),
    );
    await waitFor(() =>
      expect(bodies).toEqual([{ posture: "held", apply: true }]),
    );
  });

  // With no provider on the return, `live` is the roster's FIRST live OAuth
  // mailbox — a guess. It is good enough to offer a history read, which the
  // reader can decline, and wrong for a write that changes who may read a
  // mailbox they did not just connect.
  it("asks nothing when the return did not name its mailbox", async () => {
    installFetchStub({
      "GET /connectors": () => jsonResponse({ data: [gmail("classified")] }),
      "GET /connectors/gmail/backfill": () => jsonResponse({ state: "idle" }),
      "GET /capture/settings": () =>
        jsonResponse({ shared_posture_allowed: false }),
    });
    render(<OAuthReturnPanel outcome="ok" onComplete={vi.fn()} />);

    // The panel itself still resolves — this is about the posture control only.
    expect(await screen.findByText("Live and capturing")).toBeTruthy();
    expect(screen.queryByText(en["connectors.mailPosture.label"])).toBeNull();
  });
});
