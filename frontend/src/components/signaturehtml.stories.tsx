// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { SignatureHtml } from "./signaturehtml";

const meta = {
  title: "Patterns/Email signature",
  component: SignatureHtml,
  parameters: { layout: "padded" },
  args: {
    html: "<p><b>Mira Voss</b><br>Head of Sales<br>+49 711 555 0100</p>",
    title: "Email signature",
  },
  decorators: [
    (Story) => {
      installFetchStub({ "GET /me": meRoute({}) });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
} satisfies Meta<typeof SignatureHtml>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Rendered: Story = {};
