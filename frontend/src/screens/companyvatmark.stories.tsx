// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { VatMark } from "./companyvatmark";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The states worth reviewing are the answers a reader can get, and they differ
// in what a business may CLAIM rather than in how they look: a valid number
// with a receipt is evidence, the same number without one is only a reading, an
// invalid one is a finding, and a register that did not answer says nothing
// about the company at all.
//
// The mark is drawn beside a value here, because that is the only place it ever
// appears — reviewed on its own it would be a glyph on a blank ground, and the
// question it has to answer is whether it reads as a note on the number rather
// than as a control the row grew.

const meta: Meta<typeof VatMark> = {
  title: "Records/Company rail/VAT mark",
  component: VatMark,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof VatMark>;
type VatCheck = components["schemas"]["OrganizationVatCheck"];

const ORG_ID = "00000000-0000-7000-8000-0000000000a1";
const NUMBER = "DE811907980";
const ROUTE = `GET /organizations/${ORG_ID}/vat-check`;
const ASK = `POST /organizations/${ORG_ID}/vat-check`;

// The grant the ask gates on. Without it every story renders as a viewer who
// may not write, and the button is absent for the correct reason — the
// omission story-utils' own comment warns is invisible.
const CAN_WRITE = meRoute({ organization: ["read", "update"] });

const CHECKED: VatCheck = {
  organization_id: ORG_ID,
  vat_number: NUMBER,
  status: "valid",
  consultation_number: "WAPIAAAAXk3rN2p9",
  registered_name: "Muster Handels GmbH",
  registered_address: "Musterstraße 1, 10115 Berlin",
  checked_at: "2026-08-14T09:12:00Z",
  recorded_at: "2026-08-14T09:12:31Z",
};

// The row as the rail draws it: the label, the value, the mark. The mark has to
// sit on the value's own line without out-measuring it.
function inRow(stated: string, canAsk = true) {
  return (
    <StoryProviders locale="de">
      <div style={{ maxWidth: 340 }}>
        <span className="t-label">Register / USt-IdNr.</span>
        <div>
          {stated}
          <VatMark orgId={ORG_ID} stated={stated} canAsk={canAsk} />
        </div>
      </div>
    </StoryProviders>
  );
}

/** The whole point: a valid number WITH the receipt that makes it evidence a
 * tax authority accepts. */
export const Valid: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () => jsonResponse(CHECKED),
      [ASK]: () => new Response(null, { status: 202 }),
    });
    return inRow(NUMBER);
  },
};

/** A number the register rejects. The most common cause is an imprint copied
 * from another company's site, which is why the registered name in the panel
 * matters more here than the verdict. */
export const Invalid: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () =>
        jsonResponse({
          ...CHECKED,
          vat_number: "DE999999999",
          status: "invalid",
          consultation_number: null,
          registered_name: null,
        }),
      [ASK]: () => new Response(null, { status: 202 }),
    });
    return inRow("DE999999999");
  },
};

/** A row an older build left behind, carrying `unavailable`. Nothing writes
 * that status now — a value that is not VAT-ID shaped is recorded as invalid,
 * and a register that declines is retried rather than written — so this is the
 * version-skew case: it reads as NOT VALID, because three states is what a
 * reader can act on and a fourth word would only ask them to interpret it. */
export const StatusFromAnOlderBuild: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () => jsonResponse({ ...CHECKED, status: "unavailable" }),
      [ASK]: () => new Response(null, { status: 202 }),
    });
    return inRow(NUMBER);
  },
};

/** Never consulted. The server answers 404 and the mark offers the honest
 * absence rather than reporting a failure nobody caused. */
export const NeverConsulted: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () => jsonResponse({ title: "not found" }, 404),
      [ASK]: () => new Response(null, { status: 202 }),
    });
    return inRow(NUMBER);
  },
};

/** The number was edited after the check, so the receipt answers for a number
 * the record no longer states. The verdict alone would be a claim about the
 * number the reader is looking at, which is not the one that was consulted. */
export const NumberMovedSinceTheCheck: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () => jsonResponse(CHECKED),
      [ASK]: () => new Response(null, { status: 202 }),
    });
    return inRow("DE123456789");
  },
};

/** Asked again too soon. The floor exists because the register is a shared
 * public service consulted on one worker, and the reader is told to wait rather
 * than that something is wrong. */
export const AskedTooSoon: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () => jsonResponse(CHECKED),
      [ASK]: () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Too Many Requests",
            status: 429,
            detail:
              "Diese Nummer wurde vor wenigen Minuten geprüft. Bitte gleich noch einmal versuchen.",
          },
          429,
        ),
    });
    return inRow(NUMBER);
  },
};

/** Asked, and waiting. The button stays busy until the register replies rather
 * than until the request is accepted — the POST answers in milliseconds and the
 * answer arrives seconds later, so a button that cleared on the 202 invited a
 * second consultation for a check already running. Press it here: it refuses,
 * keeps focus, and says what it is doing. */
export const AskedAndWaiting: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      // The register never answers, which is what holds the wait open.
      [ROUTE]: () => jsonResponse(CHECKED),
      [ASK]: () => new Response(null, { status: 202 }),
    });
    return inRow(NUMBER);
  },
};

/** A viewer who may read the company and not change it. The verdict and the
 * receipt are theirs to read; consulting the register is not. */
export const WithoutTheGrantToAsk: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ organization: ["read"] }),
      [ROUTE]: () => jsonResponse(CHECKED),
    });
    return inRow(NUMBER);
  },
};
