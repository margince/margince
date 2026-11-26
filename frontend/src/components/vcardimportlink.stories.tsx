// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import type { GrantSpec } from "../testing/mefixture";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { CompanyVCardImport } from "./vcardimportlink";

/** The import verb on a company's Contacts tab, for a seat by its grants. */
function onSeat(allow: GrantSpec) {
  installFetchStub({ "GET /me": meRoute(allow) });
  return (
    <StoryProviders>
      <CompanyVCardImport companyId="c-acme" />
    </StoryProviders>
  );
}

const meta: Meta<typeof CompanyVCardImport> = {
  title: "Patterns/vCard import link",
  component: CompanyVCardImport,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CompanyVCardImport>;

// The address names the company, so the import files new contacts under it
// and Back leads to its Contacts tab.
export const ForACompany: Story = {
  render: () => onSeat({ contact: ["create", "update"] }),
  play: async ({ canvasElement }) => {
    const verb = await within(canvasElement).findByTestId("vcard-import");
    await expect(verb.getAttribute("href")).toContain("company=c-acme");
  },
};

// A seat that may not write contacts sees the verb refused, with its reason.
export const Refused: Story = {
  render: () => onSeat({ contact: ["read"] }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(canvas.getByTestId("vcard-import").tagName).toBe("BUTTON"),
    );
  },
};
