import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { Panel } from "../../design-system/panel";
import { StoryProviders } from "../../testing/story-utils";
import { WorklistRow } from "./worklist.row";
import "./worklist.css";

const incident: components["schemas"]["WorklistItem"] = {
  occurred_precision: "instant",
  id: "failed-task",
  source: "ai_work_health",
  category: "system",
  level: 4,
  consequence: "work_blocked",
  title: "",
  ai_task: "site_triage",
  cause_label: "Website triage",
  because: [],
  actions: ["open"],
  batch: { key: "system_incident", count: 12, label: "Website triage" },
};

const meta: Meta<typeof WorklistRow> = {
  title: "Records/Worklist/AI task names",
  component: WorklistRow,
  parameters: { layout: "padded" },
  args: { item: incident, position: 1, owner: "" },
  decorators: [
    (Story, context) => (
      <StoryProviders
        locale={
          context.globals.locale === "de"
            ? "de"
            : context.globals.locale === "vi"
              ? "vi"
              : "en"
        }
      >
        <Panel title="AI">
          <ol className="worklist-list">
            <li>
              <Story />
            </li>
          </ol>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof WorklistRow>;

export const English: Story = { globals: { locale: "en" } };
export const German: Story = { globals: { locale: "de" } };
export const Vietnamese: Story = { globals: { locale: "vi" } };
export const UnknownTask: Story = {
  globals: { locale: "de" },
  args: {
    item: {
      ...incident,
      ai_task: undefined,
      cause_label: undefined,
      batch: undefined,
    },
  },
};
