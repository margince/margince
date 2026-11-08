// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { userEvent, within } from "storybook/test";
import {
  type LegalEntity,
  useLegalEntityLogo,
} from "../../lib/legalentities.queries";
import { StoryProviders } from "../../testing/story-utils";
import { EntityLogo } from "./legalentitylogo";

// The logo slot of the legal entity editor: empty, stored, and its Edit menu.

const WORDMARK =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 360 120'%3E%3Crect width='360' height='120' fill='white'/%3E%3Ctext x='180' y='76' text-anchor='middle' font-family='sans-serif' font-size='48' font-weight='700' fill='%230e7490'%3EACME WIEN%3C/text%3E%3C/svg%3E";

const VIENNA: LegalEntity = {
  id: "le-vienna",
  is_default: false,
  display_name: "Acme Wien",
  legal_name: "Acme Wien GmbH",
  country: "at",
  address: {},
  member_count: 0,
  version: 1,
  created_at: "2026-10-01T08:00:00Z",
  updated_at: "2026-10-01T08:00:00Z",
};

function Slot({ entity }: Readonly<{ entity: LegalEntity }>) {
  const write = useLegalEntityLogo();
  const [version, setVersion] = useState(entity.version);
  return (
    <EntityLogo
      entity={entity}
      write={write}
      saving={false}
      version={version}
      onVersion={setVersion}
    />
  );
}

const slot = (entity: LegalEntity) => () => (
  <StoryProviders>
    <Slot entity={entity} />
  </StoryProviders>
);

const meta: Meta<typeof EntityLogo> = {
  title: "Settings/Company/Company profile/Legal entity logo",
  component: EntityLogo,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof EntityLogo>;

export const Empty: Story = { render: slot(VIENNA) };

export const Stored: Story = {
  render: slot({ ...VIENNA, logo_url: WORDMARK }),
};

export const StoredMenuOpen: Story = {
  render: slot({ ...VIENNA, logo_url: WORDMARK }),
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await page.findByRole("button", { name: "Edit logo" }),
    );
    await page.findByRole("button", { name: "Remove logo" });
  },
};

export const StoredMenuOpenDark: Story = {
  ...StoredMenuOpen,
  globals: { theme: "dark" },
};

export const StoredPhone: Story = {
  render: slot({ ...VIENNA, logo_url: WORDMARK }),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
