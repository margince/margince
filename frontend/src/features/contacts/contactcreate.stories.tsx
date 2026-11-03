// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Row } from "../../design-system/stack";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactCreateActions } from "./contactcreate";

const meta: Meta = {
  title: "Records/Contact create/List actions",
  render: () => {
    installFetchStub({ "GET /me": meRoute({ contact: ["read", "create"] }) });
    return (
      <StoryProviders>
        <Row gap="2">
          <ContactCreateActions />
        </Row>
      </StoryProviders>
    );
  },
};
export default meta;

type Story = StoryObj;

export const QuickCaptureAndNewContact: Story = {};
