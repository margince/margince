// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { PrivacyNotice } from "./confirmprivacy";
import { installFetchStub, StoryProviders } from "./story-utils";
import "./preferences.css";
import "./confirm.css";

// The page a notice link opens, and the one link a contact who asked us to
// stop still receives. It HAS NO FORM, and that absence is the design — a
// subscription box here would be the re-engagement surface that stop exists to
// prevent, reached by a message they cannot refuse. So the states worth
// reading are the ones that change what the page may SAY.

type PrivacyInformationPage = components["schemas"]["PrivacyInformationPage"];

const ALL_RIGHTS: PrivacyInformationPage["rights"] = [
  "access",
  "rectification",
  "erasure",
  "restriction",
  "objection",
  "complain_to_authority",
];

// The purposes are the installation's own published names, served as written,
// so a fixture states them the way the wire does.
const PURPOSES = [
  "Answering enquiries about our products",
  "Sending contract and invoice correspondence",
];

function notice(card: PrivacyInformationPage) {
  return () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <PrivacyNotice card={card} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof PrivacyNotice> = {
  title: "Signed out/Privacy notice",
  component: PrivacyNotice,
};
export default meta;

type Story = StoryObj<typeof PrivacyNotice>;

// The disclosure a bought list owes: how the contact was obtained said in a
// sentence rather than as the column value `purchased_or_imported`, the day it
// happened, what the data is used for, and every right they hold.
export const Purchased: Story = {
  render: notice({
    kind: "privacy_notice",
    acquired_as: "purchased_or_imported",
    acquired_at: "2026-03-04T10:00:00Z",
    purposes: PURPOSES,
    rights: ALL_RIGHTS,
  }),
};

// The door could not say when, and the installation publishes no purposes.
// Both cards disappear rather than drawing an empty one — a heading over
// nothing reads as a list that failed to load.
export const NoDateAndNoPurposes: Story = {
  render: notice({
    kind: "privacy_notice",
    acquired_as: "referral",
    rights: ["access", "erasure", "complain_to_authority"],
  }),
};

// A kind this build has never heard of. The page falls back to the honest
// general sentence instead of printing the token: a vocabulary that grows
// should not leak its spelling onto a page a stranger reads.
export const SourceThisBuildCannotName: Story = {
  render: notice({
    kind: "privacy_notice",
    acquired_as: "partner_directory_sync",
    acquired_at: "2026-05-19T08:30:00Z",
    purposes: PURPOSES,
    rights: ALL_RIGHTS,
  }),
};

// The same disclosure in the dark theme: the cards, the page ground and the
// captions are three elevations a darker palette compresses toward each other.
export const PurchasedDark: Story = {
  globals: { theme: "dark" },
  render: notice({
    kind: "privacy_notice",
    acquired_as: "purchased_or_imported",
    acquired_at: "2026-03-04T10:00:00Z",
    purposes: PURPOSES,
    rights: ALL_RIGHTS,
  }),
};
