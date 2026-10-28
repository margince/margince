// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../../../api/schema";
import type { GrantSpec } from "../../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
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
  grants = ALL_GRANTS,
  read,
  link = () => jsonResponse({ data: [] }, 201),
}: {
  list?: CandidateList;
  grants?: GrantSpec;
  read?: () => Response;
  link?: () => Response;
}) {
  installFetchStub({
    "GET /me": meRoute(grants),
    [CANDIDATES]: read ?? (() => jsonResponse(list)),
    [LINK]: link,
  });
}

function arrival(options: Parameters<typeof stub>[0] = {}) {
  return () => {
    stub(options);
    offerDomainContacts({ id: COMPANY_ID, domains: [{}] });
    return (
      <StoryProviders>
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

// The role may read the candidates but not link them: the verb stays, refused.
export const CalloutDenied: Story = {
  render: callout({
    grants: { company: ["read"], contact: ["read"], relationship: ["read"] },
  }),
};

export const CalloutDeniedPhone: Story = { ...CalloutDenied, ...phone };

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
    await expect(await screen.findByRole("dialog")).toBeVisible();
  },
};
