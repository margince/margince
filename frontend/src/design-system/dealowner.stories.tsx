// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type CSSProperties, useRef } from "react";
import { PORTRAIT, WHITE_PORTRAIT } from "../testing/seatphotos";
import { DealOwner, type DealOwnerSeat } from "./dealowner";

const row: CSSProperties = {
  display: "flex",
  gap: "var(--space-3)",
  alignItems: "center",
};

const OWNERS: readonly DealOwnerSeat[] = [
  { id: "u-1", name: "Lena Hoffmann", photo: PORTRAIT },
  { id: "u-2", name: "Jonas Becker", photo: WHITE_PORTRAIT },
  { id: "u-3", name: "Mia Wolf" },
];

function Owners() {
  const dealLink = useRef<HTMLAnchorElement>(null);
  return (
    <div style={row}>
      <a ref={dealLink} href="#/deals/d-1">
        Acme expansion
      </a>
      {OWNERS.map((owner) => (
        <DealOwner key={owner.id} owner={owner} dealLink={dealLink} />
      ))}
    </div>
  );
}

// The owner's mark on a deal card: a picture, a white one in its ring, and the
// mesh and initials of a seat with none. Its name is its label and its tip.
const meta: Meta<typeof DealOwner> = {
  title: "Components/Images and icons/Deal owner",
  component: DealOwner,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof DealOwner>;

export const WithAndWithoutPictures: Story = { render: () => <Owners /> };

export const WithAndWithoutPicturesDark: Story = {
  render: () => <Owners />,
  globals: { theme: "dark" },
};
