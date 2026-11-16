import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  MyOutcomes,
  MyOutcomesEmpty,
  MyOutcomesPhone,
} from "./analytics.stories";

const meta: Meta = { title: "Records/Reports/Own outcomes" };
export default meta;
export const Default = MyOutcomes;
export const Empty = MyOutcomesEmpty;
// Spelled here, not only spread. The story index reads a story's tags and
// globals from this file's source, and a bare re-export renders at desktop width.
export const Phone: StoryObj = {
  ...MyOutcomesPhone,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
