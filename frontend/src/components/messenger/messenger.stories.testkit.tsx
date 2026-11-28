import type { ReactNode } from "react";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  refuseUnrouted,
  StoryProviders,
} from "../../testing/story-utils";
import { messengerContext, messengerScope } from "./messenger.testkit";

export function MessengerStory({
  children,
  provider = "slack",
  manageProviders = false,
  dispactConfigured = true,
}: Readonly<{
  children: ReactNode;
  provider?: "dispact" | "slack" | "teams";
  manageProviders?: boolean;
  dispactConfigured?: boolean;
}>) {
  const path = `/messenger/slack/deal/${messengerScope.id}`;
  installFetchStub(
    {
      [`GET /deals/${messengerScope.id}`]: () =>
        jsonResponse({
          id: messengerScope.id,
          name: "Pilot rollout",
          writable: true,
        }),
      "GET /messenger/service": () =>
        jsonResponse({ providers: [provider], active_provider: provider }),
      "GET /me": meRoute(
        {
          activity: ["create"],
          capture_settings: manageProviders ? ["read", "update"] : [],
        },
        { seat: "full" },
      ),
      [`GET /channel-links/deal/${messengerScope.id}`]: () =>
        jsonResponse({ state: "none" }),
      "GET /messenger/dispact/identity": () =>
        jsonResponse({ state: "needs_consent", provider: "dispact" }),
      "GET /messenger/slack/identity": () =>
        jsonResponse({
          state: "connected",
          provider: "slack",
          workspace: "T_DEMO",
          grant_version: 1,
          sender: messengerContext.sender,
        }),
      "GET /messenger/teams/identity": () =>
        jsonResponse({ state: "unavailable", provider: "teams" }),
      "GET /messenger/dispact/setup": () =>
        jsonResponse({
          provider: "dispact",
          configured: dispactConfigured,
          callback_url: "",
          account_url: "https://chat.example.test",
          participant_scopes: ["read", "write"],
          max_group_recipients: 8,
        }),
      "POST /messenger/dispact/credential": () =>
        new Response(null, { status: 204 }),
      "GET /messenger/slack/setup": () =>
        jsonResponse({
          provider: "slack",
          configured: true,
          callback_url: "https://app.example.test/v1/messenger/slack/callback",
          participant_scopes: ["chat:write"],
          max_group_recipients: 8,
        }),
      [`GET ${path}/team`]: () =>
        jsonResponse({
          members: [
            {
              crm_id: "018f3a1b-0000-7000-8000-00000000a101",
              kind: "user",
              name: "Alex Morgan",
              member_id: "U_SELF",
              state: "self",
            },
            {
              crm_id: "018f3a1b-0000-7000-8000-00000000a102",
              kind: "user",
              name: "Jordan Lee",
              member_id: "U_JORDAN",
              state: "matched",
            },
          ],
        }),
      [`POST ${path}/outcome-draft`]: () =>
        jsonResponse({
          subject: "Legal approval required",
          body: "The pilot is awaiting Legal approval. Alex will confirm the trial date.",
        }),
      [`GET ${path}/channels`]: () =>
        jsonResponse({
          conversations: [messengerContext.conversation],
          next_cursor: "",
        }),
      [`GET ${path}/members`]: () =>
        jsonResponse({
          members: messengerContext.conversation.members.filter(
            (person) => person.id !== messengerContext.sender.id,
          ),
          next_cursor: "",
        }),
      [`GET ${path}/conversation`]: (_body, url) =>
        jsonResponse(
          url.searchParams.get("conversation_id") === "D_GROUP"
            ? {
                ...messengerContext,
                conversation: {
                  ...messengerContext.conversation,
                  id: "D_GROUP",
                  kind: "group",
                  name: "Jordan Lee",
                },
              }
            : messengerContext,
        ),
      [`POST ${path}/conversation`]: (body) => {
        const privateChat =
          typeof body === "object" &&
          body !== null &&
          ("members" in body ||
            ("conversation_id" in body && body.conversation_id === "D_GROUP"));
        return jsonResponse(
          privateChat
            ? {
                ...messengerContext,
                conversation: {
                  ...messengerContext.conversation,
                  id: "D_GROUP",
                  kind: "group",
                  name: "Jordan Lee",
                },
              }
            : messengerContext,
        );
      },
      [`GET ${path}/messages`]: () =>
        jsonResponse({
          messages: [
            {
              id: "1",
              root_id: "",
              author: "U_JORDAN",
              text: "The buyer can start the pilot on 14 June if the data processing agreement is signed by Friday. Could you ask Legal to review it? This is still a request, not a confirmed commitment.",
              at: "2026-06-01T10:00:00Z",
              edited: false,
              deleted: false,
            },
            {
              id: "2",
              root_id: "",
              author: "U_SELF",
              text: "I’ll ask Legal and confirm here when we have their answer.",
              at: "2026-06-01T10:05:00Z",
              edited: false,
              deleted: false,
            },
          ],
          next_cursor: "",
        }),
      [`POST ${path}/messages`]: (body) =>
        jsonResponse({
          id: "018f3a1b-0000-7000-8000-00000000a001",
          request_id:
            typeof body === "object" && body !== null && "request_id" in body
              ? body.request_id
              : "",
          conversation_id:
            typeof body === "object" &&
            body !== null &&
            "conversation_id" in body
              ? body.conversation_id
              : "",
          provider_message_id: "3",
          state: "accepted",
          updated_at: "2026-06-01T10:06:00Z",
        }),
    },
    refuseUnrouted,
  );
  return <StoryProviders>{children}</StoryProviders>;
}
