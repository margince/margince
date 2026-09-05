// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { FileChip } from "./filechip";

// A stored file as the card a reader clicks to get it. The three cases that
// matter are on one canvas: a PDF, which is what agreement paper arrives as,
// an image, which is what a photo or a mail signature's logo arrives as, and
// everything else, which gets the neutral mark rather than a guessed one.

const meta: Meta<typeof FileChip> = {
  title: "Design System/FileChip",
  component: FileChip,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof FileChip>;

export const Pdf: Story = {
  args: { href: "/v1/attachments/a-1", filename: "GR-2026-0092.pdf" },
};

export const Photo: Story = {
  args: {
    href: "/v1/attachments/a-4",
    filename: "site-photo.jpg",
    size: "2.4 MB",
  },
};

export const OtherKind: Story = {
  args: { href: "/v1/attachments/a-2", filename: "terms-redline.docx" },
};

// What a message's shelf of files looks like: the mix a mail actually carries,
// with the sizes that tell a signature logo from the paper it came with.
export const MailAttachments: Story = {
  render: () => (
    <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)" }}>
      <FileChip
        href="/v1/attachments/a-1"
        filename="GR-2026-0092.pdf"
        size="248 kB"
      />
      <FileChip
        href="/v1/attachments/a-5"
        filename="~WRD0005.jpg"
        size="823 bytes"
      />
      <FileChip
        href="/v1/attachments/a-2"
        filename="terms-redline.docx"
        size="61 kB"
      />
    </div>
  ),
};

// Two files on one row, which is the case the control exists for: an amendment
// beside a signed original is tellable apart before the click.
export const TwoOnOneRow: Story = {
  render: () => (
    <div style={{ display: "flex", gap: "var(--space-2)" }}>
      <FileChip href="/v1/attachments/a-1" filename="GR-2026-0092.pdf" />
      <FileChip
        href="/v1/attachments/a-2"
        filename="GR-2026-0092-annex-a.pdf"
      />
    </div>
  ),
};

// A scanner's filename truncates rather than running its row wide.
export const LongName: Story = {
  render: () => (
    <div style={{ maxWidth: 260 }}>
      <FileChip
        href="/v1/attachments/a-3"
        filename="Scan_2026-08-17_Rahmenvertrag_unterschrieben_final.pdf"
      />
    </div>
  ),
};
