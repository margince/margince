// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type { LegalEntity } from "./legalentities.queries";
import { ArchiveEntityDialog, EditEntityDialog } from "./legalentityeditor";

// The legal entity editor and the archive confirm, opened from a row's menu
// on the Legal entities card.

const BASE = {
  address: {},
  member_count: 0,
  version: 1,
  created_at: "2026-10-01T08:00:00Z",
  updated_at: "2026-10-01T08:00:00Z",
};

const DEFAULT: LegalEntity = {
  ...BASE,
  id: "le-default",
  is_default: true,
  display_name: "Acme",
  legal_name: "Acme GmbH",
  iban: "DE89370400440532013000",
  bic: "COBADEFFXXX",
};

const VIENNA: LegalEntity = {
  ...BASE,
  id: "le-vienna",
  is_default: false,
  display_name: "Acme Wien",
  legal_name: "Acme Wien GmbH",
  country: "at",
  member_count: 3,
  address: { line1: "Kärntner Ring 12", postal_code: "1010", city: "Wien" },
  vat_id: "ATU12345678",
  register_court: "Handelsgericht Wien",
  register_number: "FN 123456 a",
  default_language: "de",
  bank_account_holder: "Acme Wien GmbH",
  iban: "AT611904300234573201",
  bic: "BKAUATWW",
  imprint: "Acme Wien GmbH, Kärntner Ring 12, 1010 Wien",
  offering: {
    offer_summary: "Managed cloud operations for Austrian mid-sized firms.",
    icp: "Manufacturers with 200 to 2,000 staff in Austria.",
  },
};

const meta = {
  title: "Settings/Company/Company profile/Legal entity editor",
  component: EditEntityDialog,
  args: { entity: VIENNA, onClose: () => undefined },
} satisfies Meta<typeof EditEntityDialog>;
export default meta;
type Story = StoryObj<typeof meta>;

const editor = (entity: LegalEntity) => () => (
  <StoryProviders>
    <EditEntityDialog entity={entity} onClose={() => undefined} />
  </StoryProviders>
);

// Every field a further entity describes, its logo among them.
export const FurtherEntity: Story = { render: editor(VIENNA) };

// The default keeps only its country, language, bank details and imprint; the
// rest is the company profile's.
export const DefaultEntity: Story = { render: editor(DEFAULT) };

export const FurtherEntityDark: Story = {
  globals: { theme: "dark" },
  render: editor(VIENNA),
};

export const FurtherEntityPhone: Story = {
  globals: { viewport: { value: "phone" } },
  render: editor(VIENNA),
};

// Colleagues still work for the entity, and the refusal says what to do first.
export const ArchiveRefused: Story = {
  render: () => {
    installFetchStub({
      "POST /legal-entities/le-vienna/archive": () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Conflict",
            status: 409,
            code: "legal_entity_has_members",
            detail: "3 users still work for this legal entity",
          },
          409,
        ),
    });
    return (
      <StoryProviders>
        <ArchiveEntityDialog entity={VIENNA} onClose={() => undefined} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Archive" }),
    );
    await within(dialog).findByText(/Colleagues still work for this entity/);
  },
};
