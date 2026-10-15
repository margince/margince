import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { WebhooksCard } from "./webhooks";

// The subscription drawer, opened from the card the way an operator opens it.
// The stories cover a new subscription, an edit with sealed headers and a
// custom template, the event picker, and a refusal shown at its field.

const shaped = {
  id: "sub-shaped",
  owner_id: "u1",
  target_url: "https://hooks.slack.test/services/T000/B000/sales-wins",
  event_types: ["deal.created", "deal.stage_changed"],
  state: "active",
  custom_headers: [
    { name: "Authorization", configured: true },
    { name: "X-Tenant", configured: true },
  ],
  body_template:
    '{\n  "text": "{{ type }} on {{ entity.type }} {{ entity.id }}"\n}',
  body_content_type: "application/json",
  version: 4,
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-10-14T09:12:00Z",
  archived_at: null,
};

function routes(extra: Record<string, () => Response> = {}) {
  return {
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: {
            webhook_subscription: ["read", "create", "update", "delete"],
          },
        }),
      ),
    "GET /webhook-subscriptions": () =>
      jsonResponse({
        data: [shaped],
        page: { next_cursor: null, has_more: false },
        delivery_enabled: true,
      }),
    "POST /webhook-subscriptions/body-preview": () =>
      jsonResponse({
        body: '{\n  "text": "deal.stage_changed on deal 0190a7f2-0000-7000-8000-000000000002"\n}',
        content_type: "application/json",
        byte_length: 74,
      }),
    ...extra,
  };
}

function cardStory(extra?: Record<string, () => Response>) {
  return () => {
    installFetchStub(routes(extra));
    return (
      <StoryProviders>
        <WebhooksCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof WebhooksCard> = {
  title: "Settings/Data/Integrations/Webhook subscription drawer",
  component: WebhooksCard,
};
export default meta;
type Story = StoryObj<typeof WebhooksCard>;

export const NewSubscription: Story = {
  render: cardStory(),
  play: async () => {
    await userEvent.click(
      await screen.findByTestId("new-webhook-subscription"),
    );
  },
};

export const EditWithHeadersAndTemplate: Story = {
  render: cardStory(),
  play: async () => {
    await userEvent.click(
      await screen.findByTestId("edit-webhook-subscription"),
    );
  },
};

export const EventPickerOpen: Story = {
  render: cardStory(),
  play: async () => {
    await userEvent.click(
      await screen.findByTestId("edit-webhook-subscription"),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Add events" }),
    );
  },
};

export const ReservedHeaderRefused: Story = {
  render: cardStory({
    "PATCH /webhook-subscriptions/sub-shaped": () =>
      jsonResponse(
        {
          title: "Unprocessable Entity",
          code: "validation_error",
          details: {
            errors: [
              {
                field: "custom_headers",
                code: "invalid",
                message:
                  "Webhook-Signature is reserved: Margince sets webhook-signature",
              },
            ],
          },
        },
        422,
      ),
  }),
  play: async () => {
    await userEvent.click(
      await screen.findByTestId("edit-webhook-subscription"),
    );
    await userEvent.click(
      await screen.findByTestId("save-webhook-subscription"),
    );
  },
};
