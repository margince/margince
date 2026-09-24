// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import {
  SubscriptionConfirm,
  SubscriptionConfirmBody,
} from "./confirmsubscription";
import { explainPublicError, RateLimitedError } from "./preferences";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import "./preferences.css";
import "./confirm.css";

// The confirm link's other answer: one named subscription and nothing else.
//
// Every state here is one the page actually reaches. The ask is the resting
// one; "already subscribed" is its own surface rather than a disabled button,
// because a second click or a prefetching mail client must be told the answer
// is recorded instead of shown a form that looks broken. The refusal is the
// third, and it has to be legible beside a button somebody just pressed — this
// page returns before the record page's error line, so a rejection rendered
// nowhere is a rejection nobody sees.

type SubscriptionPage = components["schemas"]["SubscriptionConfirmationPage"];

const TOKEN = "cfm-9b21";

const card = (over: Partial<SubscriptionPage>): SubscriptionPage => ({
  kind: "subscription_confirmation",
  purpose_key: "product_updates",
  purpose_label: "Product updates",
  state: "unknown",
  ...over,
});

const meta: Meta<typeof SubscriptionConfirmBody> = {
  title: "Signed out/Confirm your details/Subscription",
  component: SubscriptionConfirmBody,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: { onConfirm: () => {}, submitting: false },
};
export default meta;

type Story = StoryObj<typeof SubscriptionConfirmBody>;

/** As the link opens it: the purpose, the sentence, and one unpressed verb. */
export const Asked: Story = { args: { card: card({}) } };

/** The same ask in the dark palette, where card and page ground compress. */
export const AskedDark: Story = {
  globals: { theme: "dark" },
  args: { card: card({}) },
};

/** The submit is in flight, so the verb refuses a second press. */
export const Submitting: Story = {
  args: { card: card({}), submitting: true },
};

/**
 * The link followed a second time. Not a disabled form — a different page,
 * saying the answer is already recorded and naming what it was about.
 */
export const AlreadySubscribed: Story = {
  args: { card: card({ state: "granted" }) },
};

/**
 * A refused submit. The wording is the public explainer's, not this file's:
 * a subject who tripped the rate limit is told to wait, never shown an
 * internal reason.
 */
function Refused() {
  const t = useT();
  return (
    <SubscriptionConfirmBody
      card={card({})}
      onConfirm={() => {}}
      submitting={false}
      error={explainPublicError(new RateLimitedError(), t)}
    />
  );
}

export const RefusedByTheRateLimit: Story = { render: () => <Refused /> };

/**
 * The wired page, end to end: the contact presses the one verb and the server
 * records it, which is what turns the ask into the already-subscribed surface.
 *
 * It is here because `SubscriptionConfirm` owns that transition — the body
 * above is handed a card and cannot show it happening.
 */
export const ConfirmingTheSubscription: Story = {
  render: () => {
    installFetchStub({
      [`POST /public/confirm/${TOKEN}`]: () => jsonResponse({}),
    });
    return <SubscriptionConfirm token={TOKEN} card={card({})} />;
  },
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Confirm subscription" }),
    );
    await screen.findByText("You are subscribed");
  },
};
