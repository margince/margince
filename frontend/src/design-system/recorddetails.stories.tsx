// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "./atoms";
import { RecordDetails } from "./recorddetails";

const meta: Meta<typeof RecordDetails> = {
  title: "Components/Text and data display/Record details",
  component: RecordDetails,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "18.25rem" }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof RecordDetails>;

/** A company's details: three groups, lists, five empty fields folded. */
export const Company: Story = {
  args: {
    sections: [
      {
        key: "about",
        label: "About",
        rows: [
          { key: "domain", term: "Domain", value: "brandt.example" },
          { key: "industry", term: "Industry", value: "Automotive" },
          { key: "size", term: "Size", value: "201–500" },
          { key: "lifecycle", term: "Lifecycle", value: "Customer" },
          { key: "address", term: "Address", value: "Munich, DE" },
          { key: "legal", term: "Legal name", value: "Add…", empty: true },
          { key: "linkedin", term: "LinkedIn", value: "Add…", empty: true },
          { key: "parent", term: "Parent", value: "Add…", empty: true },
        ],
      },
      {
        key: "ownership",
        label: "Ownership",
        rows: [
          { key: "owner", term: "Owner", value: "Lena Fischer" },
          { key: "visible", term: "Visible to", value: "Everyone" },
        ],
      },
      {
        key: "registration",
        label: "Registration",
        rows: [
          { key: "vat", term: "VAT ID", value: "Add…", empty: true },
          { key: "source", term: "Source", value: "Typed by a person" },
          { key: "created", term: "Created", value: "1 Jun 2026" },
          { key: "reg", term: "Register", value: "Add…", empty: true },
        ],
      },
      {
        key: "lists",
        label: "Lists",
        children: (
          <span>
            <Badge>Key accounts</Badge> <Badge>Q3 renewals</Badge>
          </span>
        ),
      },
    ],
    footnote:
      "View only. Ask Lena Fischer to share it, or an administrator for edit rights.",
  },
};

/** Nothing left blank: no toggle at the foot. */
export const NothingEmpty: Story = {
  args: {
    sections: [
      {
        key: "about",
        label: "About",
        rows: [
          { key: "company", term: "Company", value: "Weber Logistik" },
          { key: "title", term: "Title", value: "Head of Operations" },
        ],
      },
    ],
  },
};

/** German terms are longer than the 96px column: they wrap inside it rather
 *  than running into their values. */
export const LongGermanTerms: Story = {
  decorators: [
    (Story) => (
      <div lang="de" style={{ maxWidth: "18.25rem" }}>
        <Story />
      </div>
    ),
  ],
  args: {
    sections: [
      {
        key: "about",
        label: "Überblick",
        rows: [
          { key: "size", term: "Unternehmensgröße", value: "201–500" },
          {
            key: "register",
            term: "Handelsregisternummer",
            value: "HRB 123456",
          },
          { key: "legal", term: "Rechtsform", value: "Add…", empty: true },
        ],
      },
      {
        key: "ownership",
        label: "Zuständigkeit",
        rows: [{ key: "owner", term: "Inhaber", value: "Lena Fischer" }],
      },
    ],
    footnote:
      "Du kannst diese Firma nicht ändern. Bitte den Inhaber, sie mit dir zu teilen.",
  },
};

/** A record with nothing to show: no group draws a label, no fold is offered,
 *  and only the footnote remains. */
export const OnlyAFootnote: Story = {
  args: {
    sections: [
      {
        key: "about",
        label: "About",
        rows: [{ key: "size", term: "Size", value: "Add…", empty: true }],
      },
    ],
    footnote: "Closing a project asks for a reason.",
  },
};
