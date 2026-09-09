// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { PersonBriefCard } from "./personcards";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The relationship brief.
//
// The card is a MACHINE's reading in every state it can be in, so the indigo
// band and the disclosure badge ride the panel rather than the writer that
// answered on the day. What does vary is the writer, and the foot names it:
// `Written by Margince` over a model's prose, `Assembled from your records`
// over the composition it degrades to. The two frames below differ in that
// line and in nothing else, which is the claim worth documenting.
//
// EVERY INSTANT IS FIXED. `make fe-clock-drift` runs the suite at +200 days and
// requires the same verdict, and this card prints the moment it was generated.

type Person360 = components["schemas"]["Person360"];
type PersonBrief = components["schemas"]["PersonBrief"];

const AT = "2026-08-13T09:00:00Z";

const PERSON_ID = "3f7c1a90-0000-4000-8000-00000000c001";

const person: components["schemas"]["Person"] = {
  id: PERSON_ID,
  full_name: "Dana Buyer",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: AT,
};

// `commercial` is PRESENT and empty rather than absent, which is the whole
// distinction the band under the prose exists to keep: the section arrives with
// no deal when there is none, and arrives not at all when the reader may not
// see deals. Dropping it here would make this record claim a grant boundary it
// does not have.
const view: Person360 = {
  as_of: AT,
  person,
  sections_omitted: [],
  commercial: { role: null, committee: [] },
  claims: [],
};

const sentences: PersonBrief["sentences"] = [
  {
    text: "Dana leads fleet operations at Brandt Automotive and is the champion on the retrofit work.",
    evidence: [{ entity_type: "person", entity_id: PERSON_ID }],
  },
  {
    text: "She asked to push the retrofit review back a week and has not replied since.",
    evidence: [{ entity_type: "person", entity_id: PERSON_ID }],
  },
];

function brief(over: Partial<PersonBrief>): PersonBrief {
  return {
    person_id: PERSON_ID,
    generated_at: AT,
    generated_by: "deterministic",
    sentences,
    ...over,
  };
}

function card(value: PersonBrief | undefined) {
  return () => {
    // The card asks who is reading, to tell a commitment the viewer owes from
    // one owed to them. Without the route it reads the stub's list-shaped
    // fallback, which is a malformed session rather than a signed-in reader.
    installFetchStub({ "GET /me": meRoute({}) });
    return (
      <StoryProviders>
        <div className="record-stack" style={{ maxWidth: 720 }}>
          <PersonBriefCard brief={value} loading={false} view={view} />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof PersonBriefCard> = {
  title: "Records/Person relationship brief",
  component: PersonBriefCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof PersonBriefCard>;

// The model's own prose. The foot says so; the band and the badge would look
// the same either way.
export const ModelWritten: Story = {
  render: card(brief({ generated_by: "model" })),
};

// The same facts composed over the same records without a model. STILL
// indigo, because the reading is still the machine's — only the foot changes,
// and it is the honest place for a reader to check what produced the words.
export const Composed: Story = { render: card(brief({})) };

// A brief with nothing to say. No writer to name and no stamp, because nothing
// was assembled — the card says so rather than inventing a sentence.
export const Empty: Story = { render: card(brief({ sentences: [] })) };

// Dark: `--aiText` and `--aiLight` are a pair tuned per theme, and an indigo
// head band is where the dark accent lift shows first.
export const ModelWrittenDark: Story = {
  globals: { theme: "dark" },
  render: card(brief({ generated_by: "model" })),
};
