// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
import { CustomFieldTranslate } from "./customfields.translations";

// The action that names a field's label and options in each UI language.
const meta: Meta = {
  title: "Settings/Sales/Records and fields/Field translations",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj;

type CustomField = components["schemas"]["CustomField"];

const field = (over: Partial<CustomField> = {}): CustomField => ({
  id: "01J2Z3K4M5N6P7Q8R9S0T1U2V4",
  object: "deal",
  label: "Procurement route",
  slug: "procurement_route",
  type: "picklist",
  status: "active",
  column_name: "cf_procurement_route",
  options: ["Direct", "Reseller"],
  show_in_create: true,
  reportable: false,
  created_by: "u1",
  created_at: "2026-06-22T14:09:00Z",
  updated_at: "2026-06-22T14:09:00Z",
  version: 1,
  ...over,
});

export const Untranslated: Story = {
  render: () => <CustomFieldTranslate field={field()} />,
};

export const Translated: Story = {
  render: () => (
    <CustomFieldTranslate
      field={field({
        label_translations: { de: "Beschaffungsweg", vi: "Kênh mua hàng" },
        option_translations: {
          Direct: { de: "Direkt", vi: "Trực tiếp" },
          Reseller: { de: "Wiederverkäufer", vi: "Đại lý" },
        },
      })}
    />
  ),
};
