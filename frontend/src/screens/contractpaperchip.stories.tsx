// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ContractPaper } from "./contractpaperchip";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
// The row's own chrome (.rec-files and its label/items rows) is defined in
// company360.css, which reaches this component by class from the screen that
// hosts it — so a story mounting the chips alone has to load it.
import "./company360.css";

// The signed paper on one agreement's row: the chips it reached, and — when
// the documents endpoint said there was more — the sentence saying so.
//
// The two states here are the whole point of the component. A read that
// reached the end presents itself as the list; a read that did not must never
// do that, which is the distinction a row of legal paper cannot afford to
// lose.

type Attachment = components["schemas"]["Attachment"];

const COMPANY = "comp-1";
const CONTRACT = "contract-1";

function filed(id: string, filename: string, bytes: number): Attachment {
  return {
    id,
    entity_type: "company",
    entity_id: COMPANY,
    filename,
    byte_size: bytes,
    category: "contract",
    source: "upload",
    captured_by: "human:u1",
    created_at: "2026-03-02T09:00:00Z",
  };
}

// Named by their FILES, an original and its amendment: two links reading
// "Contract" would be two coin flips.
const SIGNED = filed("att-1", "Rahmenvertrag-2026-signed.pdf", 482_311);
const AMENDMENT = filed("att-2", "Nachtrag-1-signed.pdf", 96_004);

const complete = {
  data: [SIGNED, AMENDMENT],
  page: { next_cursor: null, has_more: false },
};

// The first page of a longer library, and the page the count walks to reach
// the end of it — three more documents this row will not show.
const firstOfMore = {
  data: [SIGNED, AMENDMENT],
  page: { next_cursor: "page-2", has_more: true },
};
const tail = {
  data: [
    filed("att-3", "Anlage-A-Leistungsschein.pdf", 22_105),
    filed("att-4", "Anlage-B-Preise.pdf", 18_440),
    filed("att-5", "Nachtrag-2-signed.pdf", 71_820),
  ],
  page: { next_cursor: null, has_more: false },
};

// The endpoint is asked once per page of the walk, and the cursor never
// reaches the handler — so the pages are served in the order the walk asks
// for them, with the last one answering any further ask.
function documents(pages: readonly unknown[]) {
  return () => {
    let served = 0;
    installFetchStub({
      [`GET /companies/${COMPANY}/documents`]: () => {
        const page = pages[Math.min(served, pages.length - 1)];
        served += 1;
        return jsonResponse(page);
      },
    });
    return (
      <StoryProviders>
        <ContractPaper contractId={CONTRACT} companyId={COMPANY} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ContractPaper> = {
  title: "Records/Company 360/Contract paper",
  component: ContractPaper,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ContractPaper>;

/** The whole of an agreement's paper: an original and one amendment. */
export const Filed: Story = { render: documents([complete]) };

/**
 * More paper than the row shows. The chips are the ones the read reached, and
 * under them the count of what it did not — never the first page presented as
 * the list.
 */
export const MoreThanTheRowShows: Story = {
  render: documents([firstOfMore, tail]),
};

// The chips carry a tinted ground and a hairline, and the remainder sentence
// sits at caption contrast: both are derived from tokens that lift on dark.
export const FiledDark: Story = {
  globals: { theme: "dark" },
  render: documents([complete]),
};
