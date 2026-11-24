import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { StoryProviders } from "../../testing/story-utils";
import {
  type EmploymentStatusChoice,
  EmploymentStatusFields,
} from "./employmentstatusfields";

// The pair as a dialog holds it: its own state, so the select and the date
// can be driven by hand.
function Fields({ initial }: Readonly<{ initial: EmploymentStatusChoice }>) {
  const [status, setStatus] = useState<EmploymentStatusChoice>(initial);
  const [end, setEnd] = useState(initial === "former" ? "2019-03" : "");
  return (
    <StoryProviders>
      <div className="form-stack" style={{ maxWidth: 420 }}>
        <EmploymentStatusFields
          status={status}
          onStatusChange={setStatus}
          end={end}
          onEndChange={setEnd}
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Fields> = {
  title: "Records/Contact 360/Employment status fields",
  component: Fields,
};
export default meta;
type Story = StoryObj<typeof meta>;

export const Current: Story = { args: { initial: "current" } };
export const Former: Story = { args: { initial: "former" } };
// An imported link nobody has judged yet: the select asks for a choice.
export const NotChosen: Story = { args: { initial: "" } };
export const FormerDark: Story = {
  args: { initial: "former" },
  globals: { theme: "dark" },
};
