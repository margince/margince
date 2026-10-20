// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LegalEntitiesCard } from "./legalentities";

// Settings › Company › the companies the installation trades as. With one
// entity the card lists the default alone, named after the company.

function entity(id: string, extra: Record<string, unknown> = {}) {
  return {
    id,
    is_default: false,
    display_name: "",
    address: {},
    member_count: 0,
    version: 1,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    ...extra,
  };
}

const DEFAULT = entity("le-default", {
  is_default: true,
  display_name: "Acme",
  legal_name: "Acme GmbH",
});
const VIENNA = entity("le-vienna", {
  display_name: "Acme Wien",
  legal_name: "Acme Wien GmbH",
  country: "at",
  member_count: 3,
});

const ADMIN = { legal_entity: ["read", "create", "update", "delete"] } as const;
const READER = { legal_entity: ["read"] } as const;

function story(allow: Parameters<typeof meRoute>[0], rows: object[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /legal-entities": () => jsonResponse({ data: rows }),
    });
    return (
      <StoryProviders>
        <LegalEntitiesCard />
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Company/Company profile/Legal entities",
  component: LegalEntitiesCard,
} satisfies Meta<typeof LegalEntitiesCard>;
export default meta;
type Story = StoryObj<typeof meta>;

// What every installation sees until it adds a second entity.
export const OneEntity: Story = { render: story(ADMIN, [DEFAULT]) };

export const TwoEntities: Story = { render: story(ADMIN, [DEFAULT, VIENNA]) };

// A seat that reads the list and is offered no new entity.
export const Reader: Story = { render: story(READER, [DEFAULT, VIENNA]) };

export const TwoEntitiesPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(ADMIN, [DEFAULT, VIENNA]),
};

export const TwoEntitiesDark: Story = {
  globals: { theme: "dark" },
  render: story(ADMIN, [DEFAULT, VIENNA]),
};
