import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Button } from "../design-system/atoms";
import { openAsk } from "../lib/urlstate";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
  visibleDialog,
} from "../testing/story-utils";
import { AskFromAddress } from "./askfromaddress";

const question = "How do we prepare a sales review?";
const meta: Meta<typeof AskFromAddress> = {
  title: "Shell/Ask from address",
  component: AskFromAddress,
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ knowledge_corpus: ["read"] }),
      "GET /knowledge/corpora": () => jsonResponse({ items: [] }),
    });
    return (
      <StoryProviders>
        <Button onClick={() => openAsk(question)}>Ask from this page</Button>
        <AskFromAddress />
      </StoryProviders>
    );
  },
};
export default meta;
type Story = StoryObj<typeof AskFromAddress>;
async function open({ canvasElement }: { canvasElement: HTMLElement }) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: "Ask from this page",
    }),
  );
  const dialog = await visibleDialog();
  await expect(within(dialog).findByRole("textbox")).resolves.toHaveValue(
    question,
  );
}
export const CarriedQuestion: Story = { play: open };
export const CarriedQuestionDark: Story = {
  globals: { theme: "dark" },
  play: open,
};
