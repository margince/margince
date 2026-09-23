// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { EvidenceVerdict, profileFieldClaim } from "./evidenceverdict";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The two verbs a human has over a machine's claim, and the states a press puts
// them in. Both write through the REAL claim builder and the real endpoints:
// a story that handed the component its own confirmPath would be reviewing a
// promise this story wrote, and the refusal below would carry words no server
// ever sends.
//
// The state worth the most here is the wait. A control that renames itself
// mid-write takes both the reader's focus and the word they pressed away from
// them, so the label holds and `aria-busy` carries the fact — which means the
// pending stories assert the RESTING name, and fail the day either button
// starts swapping its own text again.

type ProfileField = components["schemas"]["CompanyProfileField"];

const COMPANY = "o-1";

// A claim the site read made and nobody has ruled on: the only state in which
// both verbs are offered at all.
const EXTRACTED: ProfileField = {
  field: "industry",
  value: "Fahrzeugbau",
  source: "site_read",
  captured_by: "agent:deepread",
  updated_at: "2026-08-01T09:00:00Z",
  // The VERSION the row was read at, and it is not decoration: both verbs pin
  // it through `requireVersion`, which refuses a row that arrived without one.
  // Absent, every write here rejected before it left the browser — so the
  // stories about the WAIT never reached a wait and showed the refusal
  // instead of the state they were written to document.
  version: 3,
};

const CONFIRM = `POST /companies/${COMPANY}/profile-fields/${EXTRACTED.field}/confirm`;
const CORRECT = `PATCH /companies/${COMPANY}/profile-fields/${EXTRACTED.field}`;

function verdict(routes: RouteMap = {}) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <EvidenceVerdict
          companyId={COMPANY}
          claim={profileFieldClaim(COMPANY, EXTRACTED)}
          canEdit
        />
      </StoryProviders>
    );
  };
}

// A write that never lands, which is how a slow one looks for as long as a
// reader is looking at it.
const neverAnswers = () => new Promise<Response>(() => {});

const meta: Meta<typeof EvidenceVerdict> = {
  title: "Records/Company 360/Evidence verdict",
  component: EvidenceVerdict,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof EvidenceVerdict>;

/** Resting: agree with the claim, or say what the value should be instead. */
export const Resting: Story = { render: verdict() };

/** Mid-confirm. The button keeps the word that was pressed and announces the
 *  wait through `aria-busy`; a second press is swallowed rather than refused
 *  natively, so the focus stays where the reader put it. */
export const Confirming: Story = {
  render: verdict({ [CONFIRM]: neverAnswers }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(
      await canvas.findByRole("button", { name: en["evidence.confirm"] }),
    );
    const confirm = await canvas.findByRole("button", {
      name: en["evidence.confirm"],
    });
    await waitFor(() => {
      expect(confirm).toHaveAttribute("aria-busy", "true");
      expect(confirm).toHaveAttribute("aria-disabled", "true");
    });
  },
};

/** Mid-correction: the same contract on the other verb, reached the way a rep
 *  reaches it — open the field, type the right value, save. */
export const SavingCorrection: Story = {
  render: verdict({ [CORRECT]: neverAnswers }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(
      await canvas.findByRole("button", { name: en["evidence.correct"] }),
    );
    const field = await canvas.findByRole("textbox", {
      name: en["evidence.correctedValue"],
    });
    await user.clear(field);
    await user.type(field, "Automotive tier-one supply");
    await user.click(canvas.getByRole("button", { name: en["evidence.save"] }));
    const save = await canvas.findByRole("button", {
      name: en["evidence.save"],
    });
    await waitFor(() => {
      expect(save).toHaveAttribute("aria-busy", "true");
      expect(save).toHaveAttribute("aria-disabled", "true");
    });
  },
};

/** A refused write, and the claim is still there to rule on — nothing was
 *  consumed by the attempt. The server states this one as the bare sentinel
 *  `permission denied`, so the words under the verbs are the catalog's: a
 *  refusal phrased as a code names no one a reader can go to about it. */
export const Refused: Story = {
  render: verdict({
    [CONFIRM]: () =>
      jsonResponse(
        {
          code: "permission_denied",
          title: "Forbidden",
          detail: "permission denied",
        },
        403,
      ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(
      await canvas.findByRole("button", { name: en["evidence.confirm"] }),
    );
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      en["common.permissionDenied"],
    );
  },
};

/** Somebody else ruled on this claim first. The precondition both verbs send is
 *  what turns that into a refusal instead of a silent overwrite, and the reader
 *  is told what happened and what to do — the server states this one as the
 *  bare sentinel `version skew`, which names nothing a reader has met. The
 *  claim stays on the row, so the verdict can be given again after a reload. */
export const RowMovedUnderYou: Story = {
  render: verdict({
    [CONFIRM]: () =>
      jsonResponse(
        { code: "version_skew", title: "Conflict", detail: "version skew" },
        409,
      ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(
      await canvas.findByRole("button", { name: en["evidence.confirm"] }),
    );
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      en["edit.versionSkew"],
    );
  },
};
