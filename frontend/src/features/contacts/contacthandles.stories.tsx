import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import { view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { useContactAboutSections } from "./contactdetails";

type Email = components["schemas"]["ContactEmail"];

const meta: Meta = { title: "Records/Contact 360/About addresses" };
export default meta;
type Story = StoryObj;

const LONG =
  "procurement.department.vendor.onboarding@brandt-automotive-holdings.example";

const address = (id: string, value: string, position: number): Email => ({
  source: "manual",
  captured_by: "human:u-1",
  id,
  email: value,
  email_type: "work",
  is_primary: position === 0,
  position,
});

function About() {
  return (
    <RecordDetails
      sections={useContactAboutSections({
        ...view,
        contact: {
          ...view.contact,
          emails: [
            address("pe-1", LONG, 0),
            address("pe-2", "dana@brandt.example", 1),
          ],
          phones: [],
          primary_email: LONG,
        },
      })}
    />
  );
}

// An address wider than the About column breaks at the @. The mailbox name and
// the domain each read whole on a line of their own, and neither is cut.
export const LongAddressBreaksAtTheAt: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }, { seat: "full" }),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 320 }}>
          <About />
        </div>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await document.fonts.ready;
    const link = await within(canvasElement).findByRole("link", { name: LONG });
    const local = link.querySelector<HTMLElement>(".contactlink-email-local");
    const domain = link.querySelector<HTMLElement>(".contactlink-email-domain");
    if (!local || !domain)
      throw new Error("the address is not drawn in two halves");
    await expect(domain.offsetTop).toBeGreaterThan(local.offsetTop);
    for (const half of [local, domain]) {
      await expect(half.scrollWidth).toBeLessThanOrEqual(half.clientWidth);
    }
  },
};

export const LongAddressBreaksAtTheAtPhone: Story = {
  ...LongAddressBreaksAtTheAt,
  tags: ["uat-phone"],
};
