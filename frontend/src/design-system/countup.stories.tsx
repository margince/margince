// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { CountUp } from "./countup";

// A figure that arrives. The static stories below are the resting look; the
// last one is the only one that shows what this is FOR, because a count-up seen
// once at mount is indistinguishable from a number.
const meta: Meta<typeof CountUp> = {
  title: "Onboarding/Count up",
  component: CountUp,
  parameters: { layout: "centered" },
};
export default meta;

type Story = StoryObj<typeof CountUp>;

export const ASmallCount: Story = {
  args: { value: 34, locale: "en-GB" },
};

// Grouped the reader's own way, which is the whole reason the locale is a prop
// rather than a default: 146,203 and 146.203 are the same number to two
// colleagues looking at one screen.
export const GroupedForItsReader: Story = {
  args: { value: 146203, locale: "de-DE" },
};

/**
 * A count that keeps being earned, which is the case this exists for.
 *
 * It climbs from where it was rather than from zero on every arrival: watch the
 * figure continue rather than fall back and start again. Restarting is what a
 * naive count-up does on each poll, and on this screen it reads as the read
 * losing its place.
 */
function StillReading() {
  const [pages, setPages] = useState(3);
  useEffect(() => {
    const tick = setInterval(() => setPages((n) => (n > 40 ? 3 : n + 7)), 1800);
    return () => clearInterval(tick);
  }, []);
  return (
    <p style={{ font: "var(--fs-display)/1 var(--f-display)" }}>
      <CountUp value={pages} locale="en-GB" />
    </p>
  );
}

export const AsAReadProgresses: Story = {
  render: () => <StillReading />,
};
