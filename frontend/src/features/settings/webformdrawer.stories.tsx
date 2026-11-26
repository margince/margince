// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  INQUIRY_FORM,
  INQUIRY_GRANTS,
  INQUIRY_MEMBERS,
} from "../../components/websiteinquiry.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { WebFormDrawer } from "./webformdrawer";

const meta: Meta<typeof WebFormDrawer> = {
  title: "Settings/Data/Website forms/Form settings",
  component: WebFormDrawer,
};
export default meta;
type Story = StoryObj<typeof WebFormDrawer>;

function setup(linked: boolean) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(INQUIRY_GRANTS),
      "GET /users": () => jsonResponse({ data: INQUIRY_MEMBERS }),
      "GET /users/names": () => jsonResponse({ data: INQUIRY_MEMBERS }),
    });
    return (
      <StoryProviders>
        <WebFormDrawer
          form={{
            ...INQUIRY_FORM,
            handling_automation_id: linked
              ? INQUIRY_FORM.handling_automation_id
              : null,
          }}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}
export const LegacyOwner: Story = { render: setup(false) };
export const LinkedInquiryPolicy: Story = { render: setup(true) };
