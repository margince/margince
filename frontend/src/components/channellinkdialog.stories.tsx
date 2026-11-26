// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useMutation } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { installFetchStub, StoryProviders } from "../testing/story-utils";
import { LinkExistingDialog } from "./channellinkdialog";

const meta: Meta<typeof LinkExistingDialog> = {
  title: "Records/Deal 360/Link a team chat channel",
  component: LinkExistingDialog,
};
export default meta;
type Story = StoryObj<typeof LinkExistingDialog>;

// The dialog drives the caller's link write; here it resolves to nothing.
function OpenDialog() {
  const link = useMutation({
    mutationFn: async (
      _body: components["schemas"]["LinkChannelRequest"],
    ): Promise<components["schemas"]["ChannelLinkState"]> => ({
      state: "linked",
    }),
  });
  return (
    <LinkExistingDialog
      open
      onClose={() => {}}
      link={link}
      channels={[
        { slug: "deal-acme", name: "deal-acme" },
        { slug: "deal-globex", name: "deal-globex" },
      ]}
    />
  );
}

export const Open: Story = {
  render: () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <OpenDialog />
      </StoryProviders>
    );
  },
};
