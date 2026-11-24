// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProblemError } from "../lib/problem";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { Button } from "./atoms";
import { ModelWorkError, useConnectAiGate } from "./connectai";

const SETUP = "GET /installation/setup";

const meta = {
  title: "Components/AI and provenance/Connect an AI first",
} satisfies Meta;
export default meta;

function setupAnswer(configured: boolean) {
  return () =>
    jsonResponse({
      complete: configured,
      steps: [{ step: "ai_models", configured, blocking: false }],
    });
}

// The gate reads the query client, so it is called below the providers.
function AiButtonBody() {
  const gate = useConnectAiGate("story-ai-reason");
  return (
    <>
      <Button variant="ai" reasonId={gate.reasonId} onClick={() => undefined}>
        Draft with AI
      </Button>
      {gate.notice}
    </>
  );
}

function AiButton() {
  return (
    <StoryProviders>
      <AiButtonBody />
    </StoryProviders>
  );
}

/** A model is connected: the button is an ordinary press. */
export const Connected: StoryObj = {
  render: () => {
    installFetchStub({ [SETUP]: setupAnswer(true) });
    return <AiButton />;
  },
};

/** No model: the button stays, refuses the press, and links to AI settings. */
export const Unbound: StoryObj = {
  render: () => {
    installFetchStub({ [SETUP]: setupAnswer(false) });
    return <AiButton />;
  },
};

/** The server refused the press with ai_not_configured. */
export const RefusedByTheServer: StoryObj = {
  render: () => (
    <StoryProviders>
      <ModelWorkError
        error={
          new ProblemError({
            status: 409,
            code: "ai_not_configured",
            detail: "No AI model is connected.",
          })
        }
      />
    </StoryProviders>
  ),
};
