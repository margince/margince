// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../../api/schema";
import type { GrantSpec } from "../../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
  storyQueryClient,
} from "../../../testing/story-utils";
import {
  DomainContactsArrival,
  DomainContactsCallout,
  offerDomainContacts,
} from "./domaincontacts";
import {
  domainCandidates,
  fiftyDomainCandidates,
  longDomainCandidates,
} from "./domaincontacts.fixtures";

type CandidateList = components["schemas"]["CompanyLinkCandidateList"];
type Story = StoryObj;

const COMPANY_ID = "01930000-0000-7000-8000-00000000d0c1";
const CANDIDATES = `GET /companies/${COMPANY_ID}/link-candidates`;
const LINK = `POST /companies/${COMPANY_ID}/link-contacts`;

const ALL_GRANTS: GrantSpec = {
  company: ["read", "update"],
  contact: ["read", "update"],
  relationship: ["read", "create"],
};

function problem(status: number, extra: Record<string, unknown> = {}) {
  return jsonResponse(
    { type: "about:blank", title: "Refused", status, ...extra },
    status,
  );
}

function stub({
  list = { data: domainCandidates, truncated: false },
  read,
  link = () => jsonResponse({ data: [] }, 201),
}: {
  list?: CandidateList;
  read?: () => Response;
  link?: () => Response;
}) {
  installFetchStub({
    "GET /me": meRoute(ALL_GRANTS),
    [CANDIDATES]: read ?? (() => jsonResponse(list)),
    [LINK]: link,
  });
}

function arrival(options: Parameters<typeof stub>[0] = {}) {
  return () => {
    stub(options);
    const client = storyQueryClient();
    offerDomainContacts(client, { id: COMPANY_ID, domains: [{}] });
    return (
      <StoryProviders client={client}>
        <DomainContactsArrival companyId={COMPANY_ID} />
      </StoryProviders>
    );
  };
}

function callout(options: Parameters<typeof stub>[0] = {}) {
  return () => {
    stub(options);
    return (
      <StoryProviders>
        <div style={{ maxWidth: 720 }}>
          <DomainContactsCallout companyId={COMPANY_ID} />
        </div>
      </StoryProviders>
    );
  };
}

const phone: Pick<Story, "parameters" | "globals" | "tags"> = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

async function tickAndLink(name: RegExp, verb: string) {
  const user = userEvent.setup();
  const dialog = await screen.findByRole("dialog");
  await user.click(await within(dialog).findByRole("checkbox", { name }));
  await user.click(within(dialog).getByRole("button", { name: verb }));
  return dialog;
}

const meta: Meta = {
  title: "Records/Company 360/Contacts/Domain contacts",
  parameters: { layout: "padded" },
};
export default meta;

export const Candidates: Story = { render: arrival() };

export const CandidatesDark: Story = {
  ...Candidates,
  globals: { theme: "dark" },
};

export const CandidatesPhone: Story = { ...Candidates, ...phone };

export const CandidatesPhoneDark: Story = {
  ...Candidates,
  ...phone,
  globals: { ...phone.globals, theme: "dark" },
};

// The keyboard path: Tab reaches a tick, Space sets it, and the count verb follows.
export const TickByKeyboard: Story = {
  render: arrival(),
  play: async () => {
    const user = userEvent.setup();
    const dialog = await screen.findByRole("dialog");
    const anja = await within(dialog).findByRole("checkbox", { name: /Anja/ });
    for (
      let press = 0;
      press < 10 && document.activeElement !== anja;
      press++
    ) {
      await user.tab();
    }
    await user.keyboard(" ");
    await expect(anja).toBeChecked();
    await expect(
      within(dialog).getByRole("button", { name: "Link 1 contact" }),
    ).toBeEnabled();
  },
};

export const LongContent: Story = {
  render: arrival({ list: { data: longDomainCandidates, truncated: false } }),
};

export const LongContentPhone: Story = { ...LongContent, ...phone };

export const Truncated: Story = {
  render: arrival({ list: { data: fiftyDomainCandidates, truncated: true } }),
};

export const TruncatedPhone: Story = { ...Truncated, ...phone };

// The server refused the write; the dialog stays open and says so in words.
export const LinkFailed: Story = {
  render: arrival({ link: () => problem(500, { detail: "" }) }),
  play: async () => {
    const dialog = await tickAndLink(/Anja/, "Link 1 contact");
    await expect(await within(dialog).findByRole("alert")).toBeInTheDocument();
  },
};

export const LinkFailedPhone: Story = { ...LinkFailed, ...phone };

// A contact stopped being a candidate while the dialog was open.
export const ListChanged: Story = {
  render: arrival({
    link: () =>
      problem(422, {
        code: "validation_error",
        details: {
          errors: [
            {
              field: "contact_ids",
              code: "not_a_link_candidate",
              message: "not a link candidate",
            },
          ],
        },
      }),
  }),
  play: async () => {
    const dialog = await tickAndLink(/Anja/, "Link 1 contact");
    await expect(
      await within(dialog).findByText(/The list changed/),
    ).toBeVisible();
  },
};

export const Callout: Story = { render: callout() };

export const CalloutDark: Story = { ...Callout, globals: { theme: "dark" } };

export const CalloutPhone: Story = { ...Callout, ...phone };

export const CalloutTruncated: Story = {
  render: callout({ list: { data: fiftyDomainCandidates, truncated: true } }),
};

export const CalloutReadFailed: Story = {
  render: callout({ read: () => problem(500, { detail: "" }) }),
};

export const CalloutOpensDialog: Story = {
  render: callout(),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", {
        name: "Review contacts",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(dialog).toBeVisible());
  },
};
