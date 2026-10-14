// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { formatFixture } from "./import.fixtures";
import { TemplatePreview } from "./import.template";

// The template before it is downloaded: every file in import order, each
// column with its guide, custom columns in the accent.

const meta: Meta = {
  title: "Settings/Data/Data import/Template preview",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function preview() {
  installFetchStub({
    "GET /me": meRoute({ import_run: ["create", "read", "update"] }),
    "GET /imports/format": () => jsonResponse(formatFixture),
  });
  return (
    <StoryProviders>
      <TemplatePreview />
    </StoryProviders>
  );
}

export const Files: Story = { render: preview };

export const FilesDark: Story = {
  globals: { theme: "dark" },
  render: preview,
};
