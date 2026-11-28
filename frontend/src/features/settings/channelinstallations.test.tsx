// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  refuseUnrouted,
  StoryProviders,
} from "../../testing/story-utils";
import { ChannelInstallations } from "./channelinstallations";

const team = "018f3a1b-0000-7000-8000-00000000a003";
function mount(
  admin = true,
  slack = (_body: unknown) => jsonResponse({ detail: "Review setup" }, 403),
) {
  const approve = vi.fn(
    (_body: unknown) => new Response(null, { status: 204 }),
  );
  installFetchStub(
    {
      "GET /me": meRoute(
        { channel_connection: admin ? ["read", "create", "update"] : ["read"] },
        { roles: admin ? ["admin"] : ["rep"], seat: "full" },
      ),
      "GET /channel-providers/slack/setup": () =>
        jsonResponse({
          configured: true,
          callback_url:
            "https://crm.example.test/v1/channel-providers/slack/callback",
          events_url: "https://crm.example.test/v1/channel-events/slack",
          scopes: [],
        }),
      "GET /channel-providers/teams/setup": () =>
        jsonResponse({
          configured: true,
          app_id: "app-fixture",
          tenant_id: "tenant-fixture",
          events_url: "https://crm.example.test/v1/channel-events/teams/bot",
          scopes: ["ChannelMessage.Read.Group"],
          installations: admin
            ? [{ id: team, name: "Account team", team_id: "verified-team" }]
            : [],
        }),
      "POST /channel-providers/teams/connect": approve,
      "POST /channel-providers/slack/connect": slack,
    },
    refuseUnrouted,
  );
  render(
    <StoryProviders>
      <ChannelInstallations />
    </StoryProviders>,
  );
  return approve;
}

afterEach(cleanup);
describe("native shared channel installation", () => {
  it("approves only the team the admin explicitly selected", async () => {
    const approve = mount();
    const button = await screen.findByRole("button", {
      name: en["channelInstall.approve"],
    });
    expect(button).toBeDisabled();
    expect(approve).not.toHaveBeenCalled();
    fireEvent.click(
      screen.getByRole("combobox", { name: en["channelInstall.team"] }),
    );
    fireEvent.click(
      await screen.findByRole("option", {
        name: "Account team (verified-team)",
      }),
    );
    fireEvent.click(button);
    await screen.findByText(en["channelInstall.connected"]);
    expect(approve).toHaveBeenCalledTimes(1);
    expect(approve.mock.calls[0]?.[0]).toEqual({ installation_id: team });
  });
  it("keeps bot installation unavailable to a rep", async () => {
    const approve = mount(false);
    expect(
      await screen.findByRole("button", {
        name: en["channelInstall.slackConnect"],
      }),
    ).toBeDisabled();
    expect(await screen.findByText(en["channelInstall.noTeams"])).toBeVisible();
    expect(
      screen.queryByRole("button", { name: en["channelInstall.approve"] }),
    ).toBeNull();
    expect(approve).not.toHaveBeenCalled();
  });
  it("requires HTTPS legal pages before building a team app package", async () => {
    mount();
    await screen.findByText(en["channelInstall.teamsSteps"]);
    fireEvent.click(screen.getByText(en["channelInstall.package"]));
    const download = screen.getByRole("button", {
      name: en["channelInstall.download"],
    });
    expect(download).toBeDisabled();
    fireEvent.change(
      screen.getByLabelText(en["channelInstall.privacy"], { exact: false }),
      { target: { value: "http://legal.example.test/privacy" } },
    );
    fireEvent.change(
      screen.getByLabelText(en["channelInstall.terms"], { exact: false }),
      { target: { value: "https://legal.example.test/terms" } },
    );
    expect(download).toBeDisabled();
    fireEvent.change(
      screen.getByLabelText(en["channelInstall.privacy"], { exact: false }),
      { target: { value: "https://legal.example.test/privacy" } },
    );
    expect(download).toBeEnabled();
  });
});

it("records the explicitly selected Slack distribution before starting consent", async () => {
  const sent: unknown[] = [];
  mount(true, (body) => {
    sent.push(body);
    return jsonResponse({ detail: "Review setup" }, 403);
  });
  fireEvent.click(
    await screen.findByRole("combobox", {
      name: en["channelProfile.distribution"],
    }),
  );
  fireEvent.click(
    await screen.findByRole("option", {
      name: en["channelProfile.nonMarketplace"],
    }),
  );
  expect(screen.getByText(en["channelProfile.unsupported"])).toBeVisible();
  fireEvent.click(
    screen.getByRole("button", { name: en["channelInstall.slackConnect"] }),
  );
  await screen.findByText(en["channelInstall.failed"]);
  expect(sent).toEqual([{ distribution: "non_marketplace" }]);
});
