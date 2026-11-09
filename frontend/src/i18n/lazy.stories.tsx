import type { Meta, StoryObj } from "@storybook/react-vite";
import { Suspense } from "react";
import { expect, within } from "storybook/test";
import { useT } from "./index";
import { lazy } from "./lazy";

function Reading() {
  const t = useT();
  return <p>{t("analytics.sectionOverview")}</p>;
}
const DeferredReading = lazy(async () => ({ default: Reading }));
const meta: Meta = {
  title: "Shell/Deferred language copy",
  render: () => (
    <Suspense fallback={null}>
      <DeferredReading />
    </Suspense>
  ),
};
export default meta;
type Story = StoryObj;
async function ready({ canvasElement }: { canvasElement: HTMLElement }) {
  await expect(
    within(canvasElement).findByText("Overview"),
  ).resolves.toBeVisible();
}
export const Translated: Story = { play: ready };
export const TranslatedDark: Story = {
  globals: { theme: "dark" },
  play: ready,
};
