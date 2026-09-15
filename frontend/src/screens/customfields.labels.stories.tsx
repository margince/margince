// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider, useT } from "../i18n";
import { objectLabels, typeLabels } from "./customfields.labels";

function CatalogLabels() {
  const t = useT();
  return (
    <dl className="firmo">
      {Object.entries({ ...typeLabels(t), ...objectLabels(t) }).map(
        ([key, label]) => (
          <div key={key}>
            <dt>{key}</dt>
            <dd>{label}</dd>
          </div>
        ),
      )}
    </dl>
  );
}
const meta: Meta = {
  title: "Settings/Sales/Fields/Catalog labels",
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;
export const Labels: Story = { render: () => <CatalogLabels /> };
export const LabelsDark: Story = { ...Labels, globals: { theme: "dark" } };
