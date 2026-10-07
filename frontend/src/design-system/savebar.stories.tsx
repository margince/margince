import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "./atoms";
import { SaveBar } from "./savebar";

const meta: Meta<typeof SaveBar> = {
  title: "Components/Forms and input/Save bar",
  component: SaveBar,
};
export default meta;
type Story = StoryObj<typeof SaveBar>;
export const Unsaved: Story = {
  render: () => (
    <SaveBar label="Unsaved changes">
      <div className="card-actions">
        <span>You have unsaved changes</span>
        <Button>Discard</Button>
        <Button variant="primary">Save changes</Button>
      </div>
    </SaveBar>
  ),
};
export const UnsavedDark: Story = { ...Unsaved, globals: { theme: "dark" } };
