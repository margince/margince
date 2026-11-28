// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { en } from "../../i18n/en";
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { messengerContext, messengerScope } from "./messenger.testkit";
import { RecordMessenger } from "./recordmessenger";

afterEach(cleanup);

it("requires immutable discovery before opening a UUID-shaped legacy Dispact slug", async () => {
  const legacy = "018f3a1b-0000-7000-8000-00000000c099";
  const reads: string[] = [];
  const path = `/messenger/dispact/deal/${messengerScope.id}`;
  installFetchStub({
    "GET /messenger/service": () =>
      jsonResponse({ providers: ["dispact"], active_provider: "dispact" }),
    "GET /me": meRoute({}, { seat: "full" }),
    [`GET /channel-links/deal/${messengerScope.id}`]: () =>
      jsonResponse({
        state: "linked",
        link: {
          id: legacy,
          provider: "dispact",
          channel_slug: legacy,
          channel_name: "Legacy channel",
          status: "active",
          verbosity: "important",
          created_at: "2026-06-01T00:00:00Z",
          native_identity: false,
        },
      }),
    "GET /messenger/dispact/identity": () =>
      jsonResponse({
        state: "connected",
        provider: "dispact",
        workspace: "T_DEMO",
        grant_version: 1,
        sender: messengerContext.sender,
      }),
    "GET /messenger/dispact/setup": () =>
      jsonResponse({
        configured: true,
        provider: "dispact",
        callback_url: "",
        participant_scopes: [],
        max_group_recipients: 8,
      }),
    [`GET ${path}/team`]: () => jsonResponse({ members: [], complete: true }),
    [`GET ${path}/channels`]: () =>
      jsonResponse({
        conversations: [
          {
            ...messengerContext.conversation,
            id: "canonical-channel-id",
            name: "Discovered channel",
          },
        ],
        next_cursor: "",
      }),
    [`GET ${path}/conversation`]: (_body, request) => {
      reads.push(request.searchParams.get("conversation_id") ?? "");
      return jsonResponse({
        ...messengerContext,
        provider: "dispact",
        conversation: {
          ...messengerContext.conversation,
          id: "canonical-channel-id",
        },
      });
    },
    [`GET ${path}/messages`]: () =>
      jsonResponse({ messages: [], next_cursor: "" }),
  });
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <RecordMessenger
        recordType="deal"
        recordId={messengerScope.id}
        name="Pilot"
      />
    </StoryProviders>,
  );
  await user.click(screen.getByRole("button", { name: "Messenger" }));
  await user.click(
    await screen.findByRole("button", { name: en["messenger.channels"] }),
  );
  const discovered = await screen.findByRole("button", {
    name: "Discovered channel",
  });
  expect(reads).toEqual([]);
  await user.click(discovered);
  await screen.findByRole("textbox");
  expect(reads).toEqual(["canonical-channel-id"]);
});
