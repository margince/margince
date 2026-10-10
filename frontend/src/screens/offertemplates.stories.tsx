import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import { OfferTemplatesAdmin } from "./offertemplates";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "./story-utils";

const meta: Meta = {
  title: "Settings/Sales/Products and offers/Offer templates",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const template = {
  id: "t-1",
  name: "Standard DE",
  locale: "de-DE",
  is_default: true,
  layout: {},
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const templatesRead = () =>
  jsonResponse({
    data: [
      template,
      {
        ...template,
        id: "t-2",
        name: "Plain EN",
        locale: "en-US",
        is_default: false,
      },
    ],
    page: { next_cursor: null, has_more: false },
  });

// Every story here needs a principal, because the screen's write affordances are
// gated on offer template grants now. The stub REFUSES to answer an unrouted `GET /me`
// rather than guessing one — so without
// this the whole catalog captured the read-only posture and no story showed the
// editor. Named once rather than repeated per story.
const AUTHORING_ME = () =>
  jsonResponse(
    meFixture({
      allow: { offer_template: ["read", "create", "update", "delete"] },
    }),
  );

function renderTemplates() {
  installFetchStub({
    "GET /me": AUTHORING_ME,
    "GET /offer-templates": templatesRead,
  });
  return (
    <StoryProviders>
      <OfferTemplatesAdmin />
    </StoryProviders>
  );
}

export const List: Story = {
  render: renderTemplates,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("row", { name: /Plain EN/ });
  },
};

export const RowMenu: Story = {
  render: renderTemplates,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Standard DE" }),
    );
    await within(document.body).findByRole("button", {
      name: "Archive template",
    });
  },
};

// Below the fold the header verbs sit in a "More actions" menu.
async function pressHeadVerb(canvasElement: HTMLElement, name: string) {
  const canvas = within(canvasElement);
  await canvas.findByRole("table");
  if (!canvas.queryByRole("button", { name })) {
    await userEvent.click(
      await canvas.findByRole("button", { name: "More actions" }),
    );
  }
  await userEvent.click(
    await within(document.body).findByRole("button", { name }),
  );
}

// The language and the default read as words in the form, not as codes.
export const NewTemplateLanguage: Story = {
  render: renderTemplates,
  play: async ({ canvasElement }) => {
    await pressHeadVerb(canvasElement, "New template");
    const form = within(await within(document.body).findByRole("dialog"));
    await userEvent.click(form.getByRole("combobox", { name: /^Language/ }));
    await within(document.body).findByRole("option", { name: "English (US)" });
  },
};
export const Empty: Story = {
  render: () => {
    installFetchStub({
      "GET /me": AUTHORING_ME,
      "GET /offer-templates": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <OfferTemplatesAdmin />
      </StoryProviders>
    );
  },
};
export const LoadError: Story = {
  render: () => {
    installFetchStub({
      "GET /me": AUTHORING_ME,
      "GET /offer-templates": () =>
        jsonResponse(
          { title: "server error", detail: "offer templates unavailable" },
          500,
        ),
    });
    return (
      <StoryProviders>
        <OfferTemplatesAdmin />
      </StoryProviders>
    );
  },
};

// At 390px each row is a card: the name heads it and the "…" sits beside it.
export const ListPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: renderTemplates,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", {
      name: "Actions for Standard DE",
    });
  },
};
