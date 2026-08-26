import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  CalendarDays,
  Mail,
  MoreHorizontal,
  PenLine,
  Phone,
} from "lucide-react";
import { Button } from "./atoms";
import { IconAction } from "./iconaction";

// The verb whose glyph is its whole label, and the row it belongs to. Hover or
// focus one and its name appears — the same string `aria-label` carries, so a
// pointer reader and a screen reader are told the same thing.

const meta: Meta<typeof IconAction> = {
  title: "Design System/IconAction",
  component: IconAction,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof IconAction>;

export const Default: Story = {
  args: {
    label: "Call",
    icon: <Phone size={15} aria-hidden="true" />,
  },
};

// A record header the way the rule shapes it: ONE labelled primary saying what
// this page is for, then the verbs a reader already knows from their glyph, then
// the overflow for everything whose consequence has to be read before it is
// pressed. The labelled button is the widest thing in the row on purpose.
export const InARecordHeader: Story = {
  render: () => (
    <div
      style={{
        display: "flex",
        gap: "var(--space-2)",
        alignItems: "center",
        flexWrap: "wrap",
      }}
    >
      <Button variant="primary" small>
        <Mail size={15} aria-hidden="true" />
        Email
      </Button>
      <IconAction
        small
        label="Call"
        icon={<Phone size={15} aria-hidden="true" />}
      />
      <IconAction
        small
        label="See meetings"
        icon={<CalendarDays size={15} aria-hidden="true" />}
      />
      <IconAction
        small
        label="Edit"
        icon={<PenLine size={15} aria-hidden="true" />}
      />
      <IconAction
        small
        label="More actions"
        icon={<MoreHorizontal size={15} aria-hidden="true" />}
      />
    </div>
  ),
};

// Refused, and saying why. The same contract `Button.reason` carries: the press
// is refused by the reason alone, and the sentence reaches a screen reader
// through `aria-describedby` rather than through a `title` a disabled control
// would never announce.
export const Refused: Story = {
  args: {
    label: "Call",
    icon: <Phone size={15} aria-hidden="true" />,
    reason: "This contact has no phone number.",
  },
};

// Mid-write: full ink and a turning mark, never the dimmed treatment that means
// "not yours to do".
export const Pending: Story = {
  args: {
    label: "Call",
    icon: <Phone size={15} aria-hidden="true" />,
    pending: true,
  },
};
