import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { vi as viCatalog } from "../i18n/vi";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
  storyQueryClient,
} from "../testing/story-utils";
import { GeneratedText } from "./generatedtext";

const id = "11111111-1111-4111-8111-111111111111";
const original = "Send the agreement by 2026-11-12";
const translated = "Gửi thỏa thuận trước ngày 2026-11-12";
const meta = {
  title: "Components/AI and provenance/Generated text",
  component: GeneratedText,
} satisfies Meta<typeof GeneratedText>;
export default meta;

export const Vietnamese: StoryObj = {
  render: () => {
    installFetchStub({
      [`POST /activities/${id}/reader-rendering`]: () =>
        jsonResponse({
          activity_id: id,
          target_language: "vi",
          source_version: 1,
          part: "subject",
          subject: translated,
          unchanged: false,
          generated_at: "2026-11-09T00:00:00Z",
        }),
    });
    const client = storyQueryClient();
    client.setQueryData(["me"], {
      ...meFixture(),
      reader_rendering_available: true,
    });
    return (
      <StoryProviders client={client} locale="vi">
        <GeneratedText
          activityId={id}
          display={{
            locale: "vi",
            source_version: 1,
            generated_parts: ["subject"],
          }}
        >
          {original}
        </GeneratedText>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText(translated)).toBeVisible();
    await userEvent.click(
      canvas.getByRole("button", {
        name: viCatalog["readerRendering.showOriginal"],
      }),
    );
    await expect(canvas.getByText(original)).toBeVisible();
  },
};

export const NoModel: StoryObj = {
  render: () => {
    installFetchStub({});
    const client = storyQueryClient();
    client.setQueryData(["me"], {
      ...meFixture(),
      reader_rendering_available: false,
    });
    return (
      <StoryProviders client={client} locale="de">
        <GeneratedText
          activityId={id}
          display={{
            locale: "de",
            source_version: 1,
            generated_parts: ["subject"],
          }}
        >
          {original}
        </GeneratedText>
      </StoryProviders>
    );
  },
};
