// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
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

/** A company's details: four group panes, five empty fields folded behind
 *  the link under them, and the footnote pane. */
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

const editAbout = fn();

/** A reader who may change the record: About and Ownership end their label
 *  line in an Edit link named for the section; Lists has none. */
export const Editable: Story = {
  args: {
    sections: [
      {
        key: "about",
        label: "About",
        help: "What the company is and where it sits.",
        edit: { onEdit: editAbout },
        rows: [
          { key: "domain", term: "Domain", value: "brandt.example" },
          { key: "industry", term: "Industry", value: "Automotive" },
        ],
      },
      {
        key: "ownership",
        label: "Ownership",
        edit: { onEdit: fn() },
        rows: [{ key: "owner", term: "Owner", value: "Lena Fischer" }],
      },
      {
        key: "lists",
        label: "Lists",
        children: <Badge>Key accounts</Badge>,
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const link = await canvas.findByRole("button", { name: "Edit About" });
    await expect(link).toHaveTextContent("Edit");
    await expect(
      canvas.getByRole("button", { name: "Edit Ownership" }),
    ).toBeVisible();
    await expect(
      canvas.queryByRole("button", { name: "Edit Lists" }),
    ).toBeNull();
    await userEvent.click(link);
    await expect(editAbout).toHaveBeenCalledTimes(1);
  },
};

/** On a phone the Edit link still closes its label line, after the "?". */
export const EditablePhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  args: Editable.args,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const link = await canvas.findByRole("button", { name: "Edit About" });
    const line = link.parentElement?.getBoundingClientRect();
    await expect(Math.round(link.getBoundingClientRect().right)).toBe(
      Math.round(line?.right ?? 0),
    );
  },
};

/** The Edit link's touch layer stops above the first row, so a thumb on the
 *  row's top edge reaches the row. */
export const EditTouchLayerPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  args: Editable.args,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const link = await canvas.findByRole("button", { name: "Edit About" });
    const value = canvas.getByText("brandt.example");
    const layer = getComputedStyle(link, "::before");
    const top = value.getBoundingClientRect().top;
    const layerEnd =
      link.getBoundingClientRect().bottom - Number.parseFloat(layer.bottom);
    await expect(Number.parseFloat(layer.height)).toBeGreaterThanOrEqual(44);
    await expect(layerEnd).toBeLessThanOrEqual(top + 0.5);
    const hit = document.elementFromPoint(
      value.getBoundingClientRect().left + 2,
      top + 1,
    );
    await expect(value.contains(hit)).toBe(true);
  },
};

/** An editable section with every row still empty keeps its pane, so its
 *  Edit link is the way to add the first value; a read-only one folds away. */
export const EditableEmpty: Story = {
  args: {
    sections: [
      {
        key: "about",
        label: "About",
        edit: { onEdit: fn() },
        rows: [{ key: "domain", term: "Domain", value: "brandt.example" }],
      },
      {
        key: "address",
        label: "Address",
        edit: { onEdit: fn() },
        rows: [
          { key: "street", term: "Street", value: "Add…", empty: true },
          { key: "city", term: "City", value: "Add…", empty: true },
        ],
      },
      {
        key: "registration",
        label: "Registration",
        rows: [{ key: "vat", term: "VAT ID", value: "Add…", empty: true }],
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("button", { name: "Edit Address" }),
    ).toBeVisible();
    await expect(
      canvas.queryByRole("heading", { name: "Registration" }),
    ).toBeNull();
    await userEvent.click(
      canvas.getByRole("button", { name: "Show 3 empty fields" }),
    );
    await expect(await canvas.findByText("City")).toBeVisible();
  },
};
export const EditableEmptyPhone: Story = {
  ...EditableEmpty,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** The fold opened: the empty fields join their groups' panes, and the link
 *  under the panes folds them again. */
export const EmptyFieldsShown: Story = {
  args: Company.args,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Show 5 empty fields" }),
    );
    await expect(await canvas.findByText("VAT ID")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Hide empty fields" }),
    ).toHaveAttribute("aria-expanded", "true");
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
