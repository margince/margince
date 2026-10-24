import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { TaskDraft } from "../lib/activitybody";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { FollowUpTaskFields } from "./followuptaskfields";

const meta: Meta<typeof FollowUpTaskFields> = {
  title: "Patterns/Follow-up fields",
  component: FollowUpTaskFields,
};
export default meta;
type Story = StoryObj<typeof FollowUpTaskFields>;
function FollowUpFieldsStory() {
  const [value, setValue] = useState<TaskDraft | undefined>({
    subject: "Send revised pricing",
    body: "Include the agreed scope.",
    day: "",
    assigneeId: "",
  });
  return (
    <StoryProviders>
      <div className="form-stack">
        <FollowUpTaskFields value={value} onChange={setValue} />
      </div>
    </StoryProviders>
  );
}
export const UndatedTask: Story = {
  render() {
    installFetchStub({ "GET /me": meRoute({ activity: ["create"] }) });
    return <FollowUpFieldsStory />;
  },
};
export const UndatedTaskDark: Story = {
  ...UndatedTask,
  globals: { theme: "dark" },
};
