// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { digest, NOT_FOUND } from "./brief.fixtures";
import { OvernightPanel } from "./brief.rail.overnight";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// What the night shift did, as one panel of Brief's context rail: capture
// counts with doors into the records they name, what moved on the projects, and
// the one connector fact worth interrupting a morning for.
//
// Read every frame in BOTH themes with the toolbar's Theme control — the
// unhealthy-connector callout is the one that matters, because the warn family
// lifts its ink in dark.

const DIGEST_ROUTES: RouteMap = {
  "GET /me": meRoute({}),
  "GET /digest": () => jsonResponse(digest),
  "GET /projects/01a00000-0000-7000-8000-000000000001": () =>
    jsonResponse({
      id: "01a00000-0000-7000-8000-000000000001",
      name: "ERP replacement",
    }),
  "GET /projects/01a00000-0000-7000-8000-000000000002": () =>
    jsonResponse({
      id: "01a00000-0000-7000-8000-000000000002",
      name: "Depot rollout",
    }),
};

function panel(routes: RouteMap = DIGEST_ROUTES) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <div className="brief-rail" style={{ maxWidth: 320 }}>
          <OvernightPanel />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta = {
  title: "Shell/Brief overnight",
};
export default meta;
type Story = StoryObj;

// An ordinary night: what was captured, what needs a look, what moved.
export const Overnight: Story = {
  render: panel(),
};

// A source is degraded, which is the one thing here that reaches a reader
// before they visit Settings. EVERY broken connector is named, not the first —
// a reader with two dead mailboxes was being told about one of them — in
// Settings' own vocabulary, with the door to where their mailboxes live.
export const OvernightUnhealthy: Story = {
  render: panel({
    ...DIGEST_ROUTES,
    "GET /digest": () =>
      jsonResponse({
        ...digest,
        connectors: [
          {
            provider: "gmail",
            status: "reauth_required",
            last_sync_error_class: "auth",
          },
        ],
      }),
  }),
};

// THE INSTALLATION'S FIRST MORNING. /v1/digest answers 404 before the first
// nightly run — and 501 where the installation does not implement it at all —
// and the panel draws nothing for either: a row of zeros is worse than a
// missing count, because a reader cannot tell it apart from a real one. The
// rail's quiet panel carries "No overnight digest" instead, so the absence is
// still said once. An empty frame here is the pass.
export const OvernightCollapsed: Story = {
  render: panel({
    ...DIGEST_ROUTES,
    "GET /digest": () => jsonResponse(NOT_FOUND, 404),
  }),
};

// The digest read failed, which is not the same as there being none. The panel
// keeps its place and says the read failed.
export const OvernightRefused: Story = {
  render: panel({
    ...DIGEST_ROUTES,
    "GET /digest": () =>
      jsonResponse({ title: "Server error", code: "internal" }, 500),
  }),
};
