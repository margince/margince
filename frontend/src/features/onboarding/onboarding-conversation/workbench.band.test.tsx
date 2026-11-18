/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../../i18n/en";
import {
  collectingState,
  render,
  stubApi,
  VoiceHarness,
} from "./voice-act.testkit";

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const chipName = new RegExp(`^${en["ob.ai.runtimeChip"]}:`);

describe("the onboarding band", () => {
  it("names no research status and no model on a member's journey", async () => {
    stubApi();
    render(
      <VoiceHarness initial={{ ...collectingState(), memberPath: true }} />,
    );

    expect(
      await screen.findByText(en["ob.conv.voice.sceneTitle"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["ob.ai.ready"])).toBeNull();
    expect(screen.queryByRole("button", { name: chipName })).toBeNull();
  });

  it("keeps the status and the model chip on the first administrator's journey", async () => {
    stubApi();
    render(<VoiceHarness initial={collectingState()} />);

    expect(await screen.findByText(en["ob.ai.ready"])).toBeTruthy();
    expect(screen.getByRole("button", { name: chipName })).toBeTruthy();
  });
});
