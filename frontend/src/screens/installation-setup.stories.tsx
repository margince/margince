// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
import { InstallationSetup } from "./installation-setup";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// First run, both questions, in the room they are asked in. What the catalog is
// for here is the thing no unit test can check: the AI step's stage carries no
// indigo, because this installation has no model bound and indigo is a claim
// that a machine did something. The Google step, one binding later, is lit.
//
// Worth flipping the Theme control on both — the stage's ground, the light and
// the Core all resolve from tokens that move with it.
const meta: Meta<typeof InstallationSetup> = {
  title: "Onboarding/First run",
  component: InstallationSetup,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof InstallationSetup>;

function sheetRow(
  model_id: string,
  lane: "chat" | "embeddings",
  input_per_mtok: string,
  output_per_mtok: string,
) {
  return {
    provider: "gemini",
    model_id,
    lane,
    input_per_mtok,
    output_per_mtok,
    cache_read_per_mtok: "0.075",
    cache_write_per_mtok: "0.3833",
    effective_date: "2026-08-01",
  };
}

// The sheet a fresh installation is seeded with, which is both the list the two
// fields offer from and the prices the plate reads.
//
// The two ids the fields OPEN on have to be in here, or the story named for a
// priced binding draws two "no price on file" slots and says the opposite of
// its own name. They are the Gemini preset's, and the server holds the same
// pair: `SETUP_PROVIDERS` is a declared mirror of `SeedModelRates`.
const RATES = {
  data: [
    sheetRow("gemini-3.1-flash-lite", "chat", "0.10", "0.40"),
    sheetRow("gemini-3.1-pro", "chat", "1.25", "10.00"),
    sheetRow("gemini-embedding-001", "embeddings", "0.15", "0"),
  ],
};

function setup(steps: ReadonlyArray<{ step: string; configured: boolean }>) {
  return {
    "GET /me": meRoute({ ai_routing: ["read", "update"] }),
    "GET /installation/setup": () =>
      jsonResponse({
        steps: steps.map((s) => ({ ...s, blocking: true })),
      }),
    "GET /ai-model-rates": () => jsonResponse(RATES),
  };
}

// Nothing bound. The room is dark, the Core is still, and the plate under the
// two fields says what the preset binding will cost before it is written.
export const BindingTheModel: Story = {
  render: () => {
    installFetchStub(
      setup([
        { step: "ai_models", configured: false },
        { step: "google_app", configured: false },
      ]),
    );
    return (
      <StoryProviders>
        <InstallationSetup />
      </StoryProviders>
    );
  },
};

// The vendor's live catalogue, ranked. Only OpenRouter has one, so the story
// picks that vendor and the list appears under the chat field's own sheet rows.
const VENDOR_CATALOGUE = {
  provider: "openrouter",
  ranked_by: "Artificial Analysis intelligence index",
  models: [
    {
      id: "anthropic/claude-opus-5",
      display_name: "Anthropic: Claude Opus 5",
      input_per_mtok: "5",
      output_per_mtok: "25",
      rank_score: "63.1",
    },
    {
      id: "x-ai/grok-4.6",
      display_name: "xAI: Grok 4.6",
      input_per_mtok: "2",
      output_per_mtok: "6",
      rank_score: "60.9",
    },
    {
      id: "z-ai/glm-5.3-flash",
      display_name: "Z.AI: GLM 5.3 Flash",
      input_per_mtok: "0.07",
      output_per_mtok: "0.25",
      rank_score: "57.5",
    },
  ],
};

// THE LIVE LIST. Press the chat field open: the sheet's own rows come first,
// then what the vendor is serving today, and the field's hint names the measure
// that put them in that order rather than claiming a bare "top ten".
//
// The reader has to CHOOSE OpenRouter to see it: the catalogue is not read
// until a vendor with a public one is picked, because until then this
// installation has no business talking to that vendor at all.
export const TheLiveList: Story = {
  render: () => {
    installFetchStub({
      ...setup([
        { step: "ai_models", configured: false },
        { step: "google_app", configured: false },
      ]),
      "GET /ai/available-models/openrouter": () =>
        jsonResponse(VENDOR_CATALOGUE),
    });
    return (
      <StoryProviders>
        <InstallationSetup />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("radio", { name: /OpenRouter/ }),
    );
  },
};

// The vendor unreachable, which is the case that must never block first run.
// The field falls back to the sheet and the hint says so, rather than the step
// failing because somebody else's service is down.
export const TheLiveListUnavailable: Story = {
  ...TheLiveList,
  render: () => {
    installFetchStub({
      ...setup([
        { step: "ai_models", configured: false },
        { step: "google_app", configured: false },
      ]),
      "GET /ai/available-models/openrouter": () =>
        jsonResponse({ provider: "openrouter", models: [], unavailable: "unreachable" }),
    });
    return (
      <StoryProviders>
        <InstallationSetup />
      </StoryProviders>
    );
  },
};

// The same screen with the price sheet unavailable — a seat that may bind a
// model but may not read rates gets a 403, which `useAiModelCatalogue` answers
// as an empty list. Both fields still work and both slots say they have no
// price, which is the case that must never render as free.
export const NoPricesOnFile: Story = {
  render: () => {
    installFetchStub({
      ...setup([
        { step: "ai_models", configured: false },
        { step: "google_app", configured: false },
      ]),
      "GET /ai-model-rates": () =>
        jsonResponse({ title: "Forbidden", status: 403 }, 403),
    });
    return (
      <StoryProviders>
        <InstallationSetup />
      </StoryProviders>
    );
  },
};

// One step later. The model is bound, so the room is lit for the rest of first
// run and for everything after it. The platform question opens on Google, and
// its notice says the part the form cannot do: saving the app connects mail and
// calendar, while sign-in reads the same pair from the environment at startup.
export const ChoosingThePlatform: Story = {
  render: () => {
    installFetchStub(
      setup([
        { step: "ai_models", configured: true },
        { step: "google_app", configured: false },
      ]),
    );
    return (
      <StoryProviders>
        <InstallationSetup />
      </StoryProviders>
    );
  },
};

// The ignition, driven the way a reader reaches it: type a key, press Continue,
// and the screen becomes the sequence rather than the next question.
//
// The play waits the sequence out, which asserts the timeline completes but does
// NOT decide what the capture shows: `fe-uat` screenshots a play story 1.5s
// after mount and this settles at about 4s, so its still frame is mid-sequence
// by construction. `Onboarding/Ignition` holds the same sequence with the room
// around it; either way this one is watched rather than read from a PNG.
//
// The wait reads the last element's own opacity rather than sleeping, so the day
// a beat moves it still knows what "finished" means.
export const TheIgnition: Story = {
  ...BindingTheModel,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByLabelText("API key"),
      "AIza-not-a-real-key",
    );
    await userEvent.click(
      await canvas.findByRole("button", { name: "Continue" }),
    );
    const carryOn = await canvas.findByRole("button", { name: "Carry on" });
    await waitFor(
      () => {
        const settled = carryOn.closest(".ob-ig-go");
        if (settled === null || getComputedStyle(settled).opacity !== "1") {
          throw new Error("the ignition has not settled yet");
        }
      },
      { timeout: 8000 },
    );
  },
};

// The answer that needs no Google app here, which is where the honest awkwardness
// lives: the operator work is somebody else's and named, and first run still
// cannot finish, because `google_app` blocks whatever this answer is. The fields
// stay usable, because pasting an app is the only way past that step today.
export const OnMicrosoft: Story = {
  ...ChoosingThePlatform,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("radio", { name: /Microsoft 365/ }),
    );
  },
};

// Neither platform: IMAP mailboxes carrying their own credentials, entered on
// the mailbox rather than here.
export const OnNeither: Story = {
  ...ChoosingThePlatform,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("radio", { name: /Neither/ }),
    );
  },
};
