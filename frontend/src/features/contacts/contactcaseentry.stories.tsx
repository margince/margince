// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ANA, contactCase } from "../../testing/contactcase.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ContactCaseEntry } from "./contactcaseentry";

// The contact page's door to its open duplicate case, in the header's pills.
const meta: Meta = { title: "Records/Contact 360/Possible duplicates" };
export default meta;

type Story = StoryObj;

/** Three other contacts may be this person. */
export const OpenCase: Story = {
  render: () => {
    stubWithSession(
      {
        [`GET /dedupe/contact-cases/${ANA}`]: () => jsonResponse(contactCase()),
      },
      {},
    );
    return (
      <StoryProviders>
        <ContactCaseEntry contactId={ANA} />
      </StoryProviders>
    );
  },
};
