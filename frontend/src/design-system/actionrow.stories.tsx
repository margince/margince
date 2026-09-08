import type { Meta, StoryObj } from "@storybook/react-vite";
import { Pencil, RotateCcwClock, Trash2 } from "lucide-react";
import { ActionRow } from "./actionrow";
import { Button } from "./atoms";
import { IconAction } from "./iconaction";

// The row that divides its verbs: the quiet ones on the leading edge, the one
// call to action on the trailing one. The distance between the two groups is
// the whole component — it is what stops a reader pressing Discard because it
// was the button under their thumb.

const meta: Meta<typeof ActionRow> = {
  title: "Design System/ActionRow",
  component: ActionRow,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ActionRow>;

// Words on both sides, which is the shape a panel footer wants: three verbs
// whose consequence a reader has to read before pressing.
export const WithTextSecondaries: Story = {
  render: () => (
    <ActionRow
      primary={
        <Button variant="primary" small>
          Save changes
        </Button>
      }
    >
      <Button small>Discard</Button>
      <Button small>Preview</Button>
    </ActionRow>
  ),
};

// Glyphs on the leading edge, which is the shape a decision card wants: verbs a
// reader already knows from their icon, so the row's width goes to the one
// control that has something to say.
export const WithIconSecondaries: Story = {
  render: () => (
    <ActionRow
      primary={
        <Button variant="primary" small>
          Accept
        </Button>
      }
    >
      <IconAction small label="Reject" icon={<Trash2 aria-hidden />} />
      <IconAction small label="Later" icon={<RotateCcwClock aria-hidden />} />
      <IconAction small label="Edit" icon={<Pencil aria-hidden />} />
    </ActionRow>
  ),
};

// No call to action: the row is the leading group and nothing else. A surface
// with nothing to single out gets a plain row rather than an empty trailing
// slot holding space open for a control it does not have.
export const SecondariesOnly: Story = {
  render: () => (
    <ActionRow>
      <IconAction small label="Reject" icon={<Trash2 aria-hidden />} />
      <IconAction small label="Later" icon={<RotateCcwClock aria-hidden />} />
      <IconAction small label="Edit" icon={<Pencil aria-hidden />} />
    </ActionRow>
  ),
};

// One call to action and nothing beside it, which is the shape a panel with a
// single verb wants. No leading group is drawn at all: an empty one is still a
// flex item, and at a width that fits the button but not the button plus a gap
// it would take the first line and push the verb onto a second, under a line of
// air holding nothing.
export const PrimaryOnly: Story = {
  render: () => (
    <ActionRow
      primary={
        <Button variant="primary" small>
          Save changes
        </Button>
      }
    />
  ),
};

// A secondary that is PRESENT and draws nothing — the shape a queue row takes
// where the server offers it no judgements and `<DispositionVerbs/>` is one
// child returning null. It has to read exactly like `PrimaryOnly` above: the
// leading group IS drawn, because the row cannot know what a child will render,
// and `.action-row-lead:empty` then takes it off the flex line so the one verb
// keeps its own line rather than sitting under a band of air.
function NoVerbsHere() {
  return null;
}

export const SecondariesRenderNothing: Story = {
  render: () => (
    <ActionRow
      primary={
        <Button variant="primary" small>
          Save changes
        </Button>
      }
    >
      <NoVerbsHere />
    </ActionRow>
  ),
};

/**
 * At 390px, where words on both sides run the row out of line and the trail
 * drops BELOW the secondaries — still on the trailing edge, which is the reason
 * the trail is a group with an `auto` inline start margin rather than a
 * `justify-content` setting on the row. A primary that wrapped to the leading
 * edge would land exactly where the reader's thumb had been pressing Discard.
 *
 * Words rather than the glyphs above, because glyphs are what a phone-width row
 * reaches for and three of them beside one verb do not fill a phone — the
 * `WithIconSecondaries` shape holds its line here, and a story claiming a wrap
 * that never happens would document the opposite of what it renders.
 *
 * `uat-phone` is what drives the capture gate's browser to 390px: Storybook's
 * own viewport is applied by the manager, which the gate's bare `iframe.html`
 * never runs, so without the tag this would be captured at desktop width and
 * would picture the very layout it exists to rule out.
 */
export const AtPhoneWidth: Story = {
  render: () => (
    // The card's own inset, so the row wraps where it would on a real surface
    // rather than against the viewport edge. `fullscreen` keeps the catalog's
    // 2rem frame off it: 390px less two frames is not a width any reader has.
    <div style={{ padding: "var(--padCard)" }}>
      <ActionRow
        primary={
          <Button variant="primary" small>
            Save and continue
          </Button>
        }
      >
        <Button small>Discard changes</Button>
        <Button small>Compare versions</Button>
      </ActionRow>
    </div>
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
