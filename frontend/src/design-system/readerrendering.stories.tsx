import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ReaderRendering } from "./readerrendering";

const ACTIVITY = "11111111-1111-4111-8111-111111111111";
const ROUTE = `POST /activities/${ACTIVITY}/reader-rendering`;
const ORIGINAL =
  "Wir haben über die Verlängerung gesprochen. Sie möchten das Angebot bis Freitag sehen.";

const meta = {
  title: "Components/AI and provenance/Reader rendering",
  component: ReaderRendering,
} satisfies Meta<typeof ReaderRendering>;
export default meta;

function Note() {
  return (
    <StoryProviders>
      <ReaderRendering activityId={ACTIVITY} language="de">
        <p>{ORIGINAL}</p>
      </ReaderRendering>
    </StoryProviders>
  );
}

/** A German note read in English: press the button to see it rendered. */
export const Rendered: StoryObj = {
  render: () => {
    installFetchStub({
      [ROUTE]: () =>
        jsonResponse({
          activity_id: ACTIVITY,
          source_language: "de",
          target_language: "en",
          subject: "Renewal",
          body: "We talked about the renewal. They want to see the offer by Friday.",
          unchanged: false,
          model_used: "stub-model",
          generated_at: "2026-11-04T09:00:00Z",
        }),
    });
    return <Note />;
  },
};

/** A note whose language was not known turns out to be the reader's already. */
export const AlreadyInYourLanguage: StoryObj = {
  render: () => {
    installFetchStub({
      [ROUTE]: () =>
        jsonResponse({
          activity_id: ACTIVITY,
          target_language: "en",
          body: ORIGINAL,
          unchanged: true,
          generated_at: "2026-11-04T09:00:00Z",
        }),
    });
    return <Note />;
  },
};

/** An installation with no AI model says so instead of a generic failure. */
export const NoModel: StoryObj = {
  render: () => {
    installFetchStub({
      [ROUTE]: () =>
        jsonResponse(
          { code: "ai_not_configured", status: 409, detail: "no model" },
          409,
        ),
    });
    return <Note />;
  },
};
