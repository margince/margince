// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { OAuthConsent } from "./oauthconsent";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The screen where a human hands an agent their own authority. It reads its
// whole request out of the redirect FRAGMENT — every authorize param plus the
// single-use `consent` nonce the server armed against an HttpOnly cookie — so a
// story arrives the way the server sends a visitor: by setting the hash before
// the component mounts (OAuthConsent reads it once per mount).
//
// The nonce is stubbed, not earned. Nothing here holds the cookie half of the
// pair, so no story submits: the forms are rendered for review, and the flow
// itself belongs to oauthconsent.test.tsx, which drives the real POST.
//
// The rule the screen keeps, and the reason the stories split the way they do: a
// submittable form is rendered ONLY where a nonce is actually held. A terminal
// refusal hands back the request alone and presents no action at all.
const NONCE = "n-4f21c8";

// The audience param rides every fixture: `resource` is the one authorize param
// whose loss would be silent, since the flow still completes bound to the wrong
// audience.
function consentHash(overrides: Record<string, string> = {}): string {
  const params = new URLSearchParams({
    response_type: "code",
    client_id: "https://agent.example/mcp",
    redirect_uri: "https://agent.example/oauth/callback",
    scope: "read write",
    code_challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
    code_challenge_method: "S256",
    resource: "https://margince.example/mcp",
    state: "night-state",
    ...overrides,
  });
  return `#/oauth-consent?${params.toString()}`;
}

// Every scope the client could ask for, ticked by default — the screen never
// narrows to what the client happened to request.
const everyScope = {
  client_name: "Fleet Copilot",
  offline: false,
  scopes: ["read", "draft", "write", "send", "enrich"],
};

// A client asking to keep working when the human is not present. The extra line
// is the whole difference, and it is the one a reader must not miss.
const offlineClient = { ...everyScope, offline: true };

// `payload` is the consent-request read. It is routed even on the states that
// never fire it (the refusals below hold no nonce, so the query stays disabled)
// so the map reads as the screen's full surface rather than a list of what each
// story happened to need.
function story(hash: string, payload: unknown, status = 200) {
  return () => {
    globalThis.location.hash = hash;
    installFetchStub({
      // An ordinary seat, not an admin: authorizing an agent is a decision any
      // signed-in human makes about their OWN authority, and the grant it
      // creates is capped by the seat they already hold.
      "GET /me": meRoute({}, { roles: ["rep"] }),
      "GET /oauth/consent-request": () => jsonResponse(payload, status),
    });
    return (
      <StoryProviders>
        <OAuthConsent />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof OAuthConsent> = {
  title: "Signed out/Authorize access",
  component: OAuthConsent,
};
export default meta;
type Story = StoryObj<typeof OAuthConsent>;

// The consent prompt: who is asking, where the authorization is sent, and every
// scope the client could hold — each one ticked, so the human narrows rather
// than builds up. Authorize and Deny are two separate forms, so neither can be
// reached by accident from the other.
export const ConsentPrompt: Story = {
  render: story(consentHash({ consent: NONCE }), everyScope),
};

// The prompt in dark. This is a decision screen with no rail around it, so the
// card IS the page, and the checkboxes and the two submits — an Authorize that
// must read as the loud one and a Deny that must still read as a real control —
// have to stay legible against a darker ground.
export const ConsentPromptDark: Story = {
  globals: { theme: "dark" },
  render: story(consentHash({ consent: NONCE }), everyScope),
};

// Same prompt, offline access requested — the client wants to act while the
// human is away.
export const OfflineAccessRequested: Story = {
  render: story(consentHash({ consent: NONCE }), offlineClient),
};

// A TERMINAL refusal, and the one that most often cannot have the
// consent-request read either: invalid_request's likeliest cause is a client
// that went unknown, disabled or deleted, which 404s that same fetch. So this
// state is rendered ahead of the read, with copy that names no client and a way
// out of a rail-less dead end.
export const InvalidRequest: Story = {
  render: story(
    consentHash({ error: "invalid_request" }),
    { title: "Not Found", status: 404 },
    404,
  ),
};

// The request is spent. No nonce comes back, so nothing here could be submitted
// — and a state with no working action presents none. The consent-request read
// is routed as the same 404 for the same reason: this state must render without
// it.
export const StaleConsent: Story = {
  render: story(
    consentHash({ error: "stale_consent" }),
    { title: "Not Found", status: 404 },
    404,
  ),
};
