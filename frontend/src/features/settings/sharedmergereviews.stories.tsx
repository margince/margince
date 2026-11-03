// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { SharedMergeReviewsCard } from "./sharedmergereviews";

// Settings, Roles and permissions: whether an owner of one duplicate may merge the others.
const meta: Meta = {
  title: "Settings/People/Roles and permissions/Shared merge reviews",
};
export default meta;

type Story = StoryObj;

function card(
  enabled: boolean,
  identity: { roles?: string[] } = {},
  locks: components["schemas"]["SettingLocks"] = [],
) {
  stubWithSession(
    { "GET /dedupe/settings": () => jsonResponse({ enabled, locks }) },
    { installation_settings: ["read", "update"] },
    identity,
  );
  return (
    <StoryProviders>
      <SharedMergeReviewsCard />
    </StoryProviders>
  );
}

/** An admin, with the setting off as it ships. */
export const Off: Story = { render: () => card(false) };

/** An admin, with the setting on. */
export const On: Story = { render: () => card(true) };

/** A rep reads the setting and cannot change it. */
export const ReadOnly: Story = {
  render: () => card(true, { roles: ["rep"] }),
};

/** The installation's configuration holds the setting. */
export const Locked: Story = {
  render: () =>
    card(false, {}, [
      {
        key: "contacts.shared_merge_reviews",
        origin: { source: "env", name: "CONTACTS_SHARED_MERGE_REVIEWS" },
      },
    ]),
};
