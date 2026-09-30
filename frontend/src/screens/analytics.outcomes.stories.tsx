import type { Meta } from "@storybook/react-vite";
import {
  MyOutcomes,
  MyOutcomesEmpty,
  MyOutcomesPhone,
} from "./analytics.stories";

const meta: Meta = { title: "Records/Reports/Own outcomes" };
export default meta;
export const Default = MyOutcomes;
export const Empty = MyOutcomesEmpty;
export const Phone = MyOutcomesPhone;
