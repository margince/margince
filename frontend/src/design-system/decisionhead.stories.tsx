import type { Meta, StoryObj } from "@storybook/react-vite";
import { commercialApproval } from "../testing/commercial-approval";
import { DecisionHead, readPayload } from "./decisionhead";

const approval = commercialApproval();
const meta: Meta<typeof DecisionHead> = {
  title: "Components/AI and provenance/Decision card/Heading",
  component: DecisionHead,
  parameters: { layout: "padded" },
  args: {
    approval,
    draft: readPayload(approval, "row", []).draft,
    headingId: "commercial-question",
    labels: {
      accept: "Approve",
      edit: "Edit",
      reject: "Reject",
      expired: "Expired",
      draftSubject: "Subject",
      draftBody: "Message",
      summaryWords: {
        field: (name) => name,
        more: (count) => `${count} more not shown`,
      },
      noContent: "No proposal content",
      loading: "Reading the proposal",
    },
  },
};
export default meta;
type Story = StoryObj<typeof DecisionHead>;

export const RecordAndReason: Story = {};
export const Compact: Story = { args: { compact: true } };
export const RecordAndReasonDark: Story = {
  globals: { theme: "dark" },
};
