/** @vitest-environment happy-dom */

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { viewerZone } from "../../format/timezone";
import { renderWithQueryClient } from "../../testing/queryrender";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../testing/story-utils";
import { OpenActivityDrawer } from "./activitydrawer";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});
it.each([403, 404])(
  "explains unavailable activity after a %s read",
  async (status) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /activities/source": () =>
        jsonResponse(
          {
            type: `urn:margince:problem:${status === 403 ? "permission_denied" : "not_found"}`,
            status,
            code: status === 403 ? "permission_denied" : "not_found",
            title: "Unavailable",
          },
          status,
        ),
    });
    renderWithQueryClient(
      <OpenActivityDrawer
        activityId="source"
        zone={viewerZone()}
        onClose={() => {}}
      />,
    );
    expect(
      await screen.findByText("Activity content is withheld"),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: /retry/i })).toBeNull();
  },
);
it("labels archived evidence while keeping its permitted text", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/source": () =>
      jsonResponse({
        id: "source",
        kind: "note",
        subject: "Pricing discussion",
        body: "The buyer asked for a revised quote.",
        occurred_at: "2026-07-01T09:00:00Z",
        archived_at: "2026-07-02T09:00:00Z",
        content_state: "available",
        links: [],
      }),
  });
  renderWithQueryClient(
    <OpenActivityDrawer
      activityId="source"
      zone={viewerZone()}
      onClose={() => {}}
    />,
  );
  expect(await screen.findByText("Archived")).toBeTruthy();
  expect(screen.getByText("The buyer asked for a revised quote.")).toBeTruthy();
});
