// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { CompanyLogo } from "./companylogo";

const meta: Meta<typeof CompanyLogo> = {
  title: "Design System/CompanyLogo",
  component: CompanyLogo,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof CompanyLogo>;

const WORDMARK =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 700 200'%3E%3Crect width='700' height='200' fill='white'/%3E%3Ctext x='350' y='125' text-anchor='middle' font-family='sans-serif' font-size='92' font-weight='700' fill='%23ff6500'%3EGRADION%3C/text%3E%3C/svg%3E";
const SQUARE_MARK =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 200 200'%3E%3Crect width='200' height='200' rx='36' fill='%2300705a'/%3E%3Cpath d='M55 105l30 30 60-70' fill='none' stroke='white' stroke-width='18' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E";

export const WideWordmark: Story = {
  args: { name: "Gradion", src: WORDMARK, fallback: "Gradion" },
  render: (args) => (
    <CompanyLogoFrame>
      <CompanyLogo {...args} />
    </CompanyLogoFrame>
  ),
};

export const SquareMark: Story = {
  args: { name: "Example company", src: SQUARE_MARK, fallback: "E" },
  render: (args) => (
    <CompanyLogoFrame>
      <CompanyLogo {...args} />
    </CompanyLogoFrame>
  ),
};

export const MissingImage: Story = {
  args: { name: "Gradion", fallback: "Gradion" },
  render: (args) => (
    <CompanyLogoFrame>
      <CompanyLogo {...args} />
    </CompanyLogoFrame>
  ),
};

function CompanyLogoFrame({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <div
      style={{
        width: 320,
        height: 100,
        border: "1px solid var(--borderSubtle)",
      }}
    >
      {children}
    </div>
  );
}
