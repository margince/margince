// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useT } from "../i18n";
import { BillingContactModal } from "./billingcontactmodal";
import type { BillingContact } from "./billingcontacts";
import { useBillingContactActions } from "./billingcontacts.queries";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// One dialog answering two questions, and the difference between them is the
// whole reason to read these side by side: naming somebody SEARCHES for the
// contact, changing a capacity shows the contact fixed. A role change keeps
// the edge and moves it, so offering a picker there would invite a reader to
// change the one thing this write cannot.

const COMPANY_ID = "01930000-0000-7000-8000-0000000000o1";

const PAT: BillingContact = {
  relationship_id: "01930000-0000-7000-8000-0000000000r1",
  contact_id: "01930000-0000-7000-8000-0000000000c1",
  full_name: "Pat Okafor",
  role: "recipient",
  title: "Office Manager",
  email: "pat@acme.test",
};

// The panel's own writes, taken from the hook that owns them rather than
// rebuilt here: the modal's pending, error and refusal states are whatever
// those mutations report, and a second set assembled for the catalog would be
// a second answer to the same question.
function OpenDialog({ editing }: Readonly<{ editing?: BillingContact }>) {
  const t = useT();
  const actions = useBillingContactActions(
    COMPANY_ID,
    t("billing.versionUnresolved"),
  );
  return (
    <BillingContactModal
      open
      onClose={() => {}}
      actions={actions}
      editing={editing}
    />
  );
}

// A colleague who may name and move billing contacts — the only seat from
// which either verb opens this dialog.
function dialog(editing?: BillingContact) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ relationship: ["create", "update", "delete"] }),
    });
    return (
      <StoryProviders>
        <OpenDialog editing={editing} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof BillingContactModal> = {
  title: "Records/Company 360/Billing contact modal",
  component: BillingContactModal,
};
export default meta;

type Story = StoryObj<typeof BillingContactModal>;

// Naming somebody: the picker is empty and Save is refused until a contact is
// chosen, because a capacity with nobody in it names nothing.
export const NamingSomebody: Story = { render: dialog() };

// Changing a capacity: the contact is stated rather than searched for, and the
// Select opens on the role they hold today.
export const ChangingCapacity: Story = { render: dialog(PAT) };

// The same two surfaces in the dark theme, where the modal ground and the
// fields inside it compress toward each other.
export const NamingSomebodyDark: Story = {
  globals: { theme: "dark" },
  render: dialog(),
};
