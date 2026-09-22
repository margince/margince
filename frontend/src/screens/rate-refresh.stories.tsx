// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useT } from "../i18n";
import { RefreshFromSources } from "./rate-refresh";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// ASKING THE PRODUCT TO RE-READ A PRICE SHEET, on its own.
//
// The control is one span carrying a button and, beside it, whatever the last
// press produced. Both screens that mount it — the currency sheet and the model
// lanes — put it in a Panel's action band, so that is the frame here: on its own
// in the canvas it would be a button with nothing to sit against, and the wrap
// the band gives it is half of why the control is shaped this way.
//
// What the three frames are about is the SPAN, not the button: the job runs in
// the background and there is nothing to poll, so the only report a reader ever
// gets is the line that appears next to the control they pressed.

const FX_REFRESH = "POST /fx-rates/propose-refresh";

// The button stays pressable after a success, which is the point of the
// `pending` state below: what stops a second refresh is the write being in
// flight, never a control the reader has lost.
function enqueued() {
  installFetchStub({
    [FX_REFRESH]: () => jsonResponse({ status: "enqueued" }, 202),
  });
}

function refused() {
  installFetchStub({
    [FX_REFRESH]: () =>
      jsonResponse(
        {
          type: "https://errors.gradion.com/forbidden",
          title: "Forbidden",
          status: 403,
          code: "permission_denied",
          detail: "Refreshing rates needs the rates grant.",
        },
        403,
      ),
  });
}

// Never answers, which is the only way to draw the button mid-write: `pending`
// is `isPending` rather than a prop, so it exists for exactly as long as a
// request is out.
function neverSettles() {
  installFetchStub({ [FX_REFRESH]: () => new Promise<Response>(() => {}) });
}

function band(stub: () => void) {
  return () => {
    stub();
    return (
      <StoryProviders>
        <Band />
      </StoryProviders>
    );
  };
}

// The band the two sheets hand it, with the panel around it so the control has
// the edge it aligns to. The title is the FX sheet's own, because that is one of
// the two places a reader meets this.
function Band() {
  const t = useT();
  return (
    <Panel
      title={t("settings.rates.fxTitle")}
      actions={<RefreshFromSources path="/fx-rates/propose-refresh" />}
    >
      <PanelBody>
        <PanelIntro>{t("settings.rates.fxIntro")}</PanelIntro>
      </PanelBody>
    </Panel>
  );
}

const meta: Meta<typeof RefreshFromSources> = {
  title: "Settings/Across pages/Refresh from sources",
  component: RefreshFromSources,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof RefreshFromSources>;

const press: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "Refresh from sources" }),
  );
};

/** At rest: one ghost verb in the band, and nothing claimed beside it. */
export const AtRest: Story = {
  render: band(enqueued),
};

/**
 * Enqueued, which is the whole of what this control can promise.
 *
 * The refresh runs in the background and stages proposals into the approvals
 * inbox, so the line says the job was accepted rather than that any rate moved —
 * and it is spoken (`role="status"`) because the button it sits beside does not
 * change to report it.
 */
export const Enqueued: Story = {
  render: band(enqueued),
  play: press,
};

/**
 * Refused, in the server's own words, and the retry is the button beside it.
 *
 * The verb stays live: a failure the reader can do nothing about would be a
 * second control, and this one already is the retry.
 */
export const Refused: Story = {
  render: band(refused),
  play: press,
};

/**
 * The write in flight. `pending`, never `disabled` — a natively disabled button
 * drops the focus of the reader who just pressed it and announces nothing.
 */
export const RefreshInFlight: Story = {
  render: band(neverSettles),
  play: press,
};
