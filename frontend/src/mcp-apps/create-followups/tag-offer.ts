// The tag-offer panel: a word the assistant proposed for the record it just
// created. Accepting applies it through the host, coining the word first when
// the workspace has none and the seat may; declining makes no call. A word
// applied can be taken off again, because that is what makes it safe to offer.

import { askAssistant, callServerTool, canCallTools } from "../actions";
import { el } from "../bridge";
import { button, panel, panelBody } from "../parts";
import { asRecord, asText } from "../types";

/** The record types a tag can sit on; the offer is not drawn for any other. */
const TAGGABLE = new Set(["contact", "company", "deal", "lead", "project"]);

type Offer = {
  name: string;
  tagID: string;
  exists: boolean;
  mayCreate: boolean;
};

type Outcome =
  | { kind: "busy" }
  | { kind: "applied" }
  | { kind: "declined" }
  | { kind: "failed"; reason: string };

const decided = new WeakMap<HTMLElement, Outcome>();

function offerOf(created: Record<string, unknown>): Offer | null {
  const raw = asRecord(created.tag_offer);
  const name = asText(raw.name);
  if (name === "" || !TAGGABLE.has(asText(created.record_type))) return null;
  return {
    name,
    tagID: asText(raw.tag_id),
    exists: raw.exists === true,
    mayCreate: raw.may_create === true,
  };
}

/** tagOfferPanel answers the panel for one create result, or null when the
 *  result carries no offer to draw. */
export function tagOfferPanel(
  root: HTMLElement,
  created: Record<string, unknown>,
  again: () => void,
): HTMLElement | null {
  const offer = offerOf(created);
  if (offer === null) return null;
  const record = {
    record_type: asText(created.record_type),
    record_id: asText(created.id),
  };
  const set = (next: Outcome) => {
    decided.set(root, next);
    again();
  };
  const card = panel("Tag this record?", { level: "h2" });
  const body = panelBody();
  body.append(
    el("p", "intro", `Tag it “${offer.name}”?`),
    choices(offer, record, decided.get(root), set),
  );
  card.appendChild(body);
  return card;
}

function choices(
  offer: Offer,
  record: { record_type: string; record_id: string },
  outcome: Outcome | undefined,
  set: (next: Outcome) => void,
): HTMLElement {
  if (outcome?.kind === "declined") return line("Not tagged.");
  if (outcome?.kind === "applied") {
    return line(
      `Tagged “${offer.name}”.`,
      button("Undo", "ghost", () => void undo(offer, record, set)),
    );
  }
  const row = el("div", "choices");
  if (!offer.exists && !offer.mayCreate) {
    row.appendChild(
      el(
        "p",
        "refusal",
        `The workspace has no tag called “${offer.name}”, and your seat cannot add one.`,
      ),
    );
    return row;
  }
  if (!canCallTools()) {
    row.appendChild(
      button("Ask the assistant to tag it", "primary", () =>
        askAssistant(
          `Tag the ${record.record_type} ${record.record_id} “${offer.name}”${offer.exists ? "" : ", adding the tag first"}.`,
        ),
      ),
    );
    return row;
  }
  const busy = outcome?.kind === "busy";
  row.append(
    button(
      offer.exists ? "Tag it" : "Add the tag and tag it",
      "primary",
      () => void accept(offer, record, set),
      busy,
    ),
    button("No thanks", "ghost", () => set({ kind: "declined" }), busy),
  );
  if (outcome?.kind === "failed") {
    row.appendChild(el("p", "refusal", outcome.reason));
  }
  return row;
}

async function accept(
  offer: Offer,
  record: { record_type: string; record_id: string },
  set: (next: Outcome) => void,
): Promise<void> {
  set({ kind: "busy" });
  if (!offer.exists) {
    const made = await callServerTool("create_tag", { name: offer.name });
    if (!made.ok) {
      set({ kind: "failed", reason: made.reason });
      return;
    }
  }
  const applied = await callServerTool("apply_tag", {
    ...record,
    ...(offer.tagID === ""
      ? { tag_name: offer.name }
      : { tag_id: offer.tagID }),
  });
  set(
    applied.ok
      ? { kind: "applied" }
      : { kind: "failed", reason: applied.reason },
  );
}

async function undo(
  offer: Offer,
  record: { record_type: string; record_id: string },
  set: (next: Outcome) => void,
): Promise<void> {
  set({ kind: "busy" });
  const removed = await callServerTool("remove_tag", {
    ...record,
    ...(offer.tagID === ""
      ? { tag_name: offer.name }
      : { tag_id: offer.tagID }),
  });
  set(
    removed.ok
      ? { kind: "declined" }
      : { kind: "failed", reason: removed.reason },
  );
}

function line(text: string, extra?: HTMLElement): HTMLElement {
  const row = el("div", "settled");
  row.appendChild(el("p", undefined, text));
  if (extra !== undefined) row.appendChild(extra);
  return row;
}
