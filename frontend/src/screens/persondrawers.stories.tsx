import type { Meta, StoryObj } from "@storybook/react-vite";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { PersonResearchDrawer } from "./persondrawers";
import "./person360.css";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The research drawer's own gallery. The tab gallery lives in
// personresearch.stories.tsx; this is the WIDE drawer a rep works inside — the
// ready state where a public-source run has staged prose claims, and each is
// mapped onto a profile field before it can be saved.

const meta: Meta<typeof PersonResearchDrawer> = {
  title: "Records/Person record/Research drawer",
  component: PersonResearchDrawer,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof PersonResearchDrawer>;

// One claim whose source a reader can cite (an http link with a verbatim
// quote), and one whose crawl returned no citable source — the second shows the
// row a reader must supply evidence for before it can be saved.
const readyRun = {
  person_id: "p-1",
  state: "ready",
  provider_name: "Clearbit",
  generated_at: "2026-08-18T09:00:00Z",
  sources_read: 4,
  claims: [
    {
      ordinal: 1,
      body: "Head of Procurement at Brandt Automotive GmbH.",
      confidence: "high",
      sources: [
        {
          label: "Brandt team page",
          url: "https://brandt-automotive.example/team",
          quote: "Dana Buyer, Head of Procurement",
        },
      ],
    },
    {
      ordinal: 2,
      body: "Speaking at the Berlin fleet-electrification summit in November.",
      confidence: "medium",
      sources: [{ label: "Conference agenda", url: "not-a-web-url" }],
    },
  ],
};

function drawer() {
  return (
    <StoryProviders>
      <ToastProvider>
        <PersonResearchDrawer
          personId="p-1"
          personName="Dana Buyer"
          open
          onClose={() => undefined}
        />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>
  );
}

/** A provider is bound and the run staged claims: each carries its prose, its
 *  sources, and the picker that turns it into a stored profile value. */
export const Ready: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ person: ["read"] }),
      "POST /people/p-1/research": () => jsonResponse(readyRun),
    });
    return drawer();
  },
};

/** No research provider is connected, so nothing was read — the honest empty
 *  state, distinct from a provider that looked and found nothing. */
export const NotConnected: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ person: ["read"] }),
      "POST /people/p-1/research": () =>
        jsonResponse({
          person_id: "p-1",
          state: "not_connected",
          generated_at: "2026-08-18T09:00:00Z",
          claims: [],
        }),
    });
    return drawer();
  },
};

/** The run is in flight: a provider was asked and has not yet answered, so the
 *  drawer says it is reading rather than showing an empty result. */
export const Loading: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ person: ["read"] }),
      // A request that never settles holds the query in its loading state.
      "POST /people/p-1/research": () => new Promise<Response>(() => {}),
    });
    return drawer();
  },
};

// A long run: enough staged claims, some with long bodies, that the drawer body
// must scroll under a pinned head and foot. This is the overflow case — the
// footer's Save must stay reachable however many claims a run returns.
const longClaims = Array.from({ length: 9 }, (_, index) => ({
  ordinal: index + 1,
  body:
    index % 3 === 0
      ? "Leads the procurement function across the DACH region, with sign-off on fleet contracts above six figures and a seat on the sustainability steering group that sets the electrification targets."
      : `Cited fact ${index + 1} about the contact, read from a public source.`,
  confidence: index % 2 === 0 ? "high" : "medium",
  sources: [
    {
      label: `Source ${index + 1}`,
      url: `https://example.com/source-${index + 1}`,
      quote: `Verbatim passage ${index + 1} the claim was read from.`,
    },
  ],
}));

/** Many staged claims, some long: the body scrolls under a pinned head and foot
 *  so the Save action never leaves the viewport. */
export const ManyClaims: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ person: ["read"] }),
      "POST /people/p-1/research": () =>
        jsonResponse({
          person_id: "p-1",
          state: "ready",
          provider_name: "Clearbit",
          generated_at: "2026-08-18T09:00:00Z",
          sources_read: 12,
          claims: longClaims,
        }),
    });
    return drawer();
  },
};
