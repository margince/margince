// The tag-offer panel: a word the assistant proposed for the record it just
// created. Accepting applies it through the host, coining the word first when
// the workspace has none and the seat may; declining makes no call. A word
// applied can be taken off again, because that is what makes it safe to offer.
//
// The tag is held by ID from the moment it exists (offered or coined), so a
// retry after a failed apply never coins it twice and an undo survives a rename.

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

/** What the panel knows: where the offer stands, and the tag's id once there is
 *  one. `reason` is what the last failed call said. */
type State = {
  phase: "idle" | "busy" | "coined" | "applied" | "declined" | "unknown";
  tagID: string;
  reason?: string;
};

const decided = new WeakMap<HTMLElement, State>();

type Target = { record_type: string; record_id: string };

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
  const state = decided.get(root) ?? { phase: "idle", tagID: offer.tagID };
  const set = (next: State) => {
    decided.set(root, next);
    again();
  };
  const card = panel("Tag this record?", { level: "h2" });
  const body = panelBody();
  body.append(
    el("p", "intro", `Tag it “${offer.name}”?`),
    choices(offer, record, state, set),
  );
  card.appendChild(body);
  return card;
}

function choices(
  offer: Offer,
  record: Target,
  state: State,
  set: (next: State) => void,
): HTMLElement {
  if (state.phase === "unknown") return line(state.reason ?? "");
  if (state.phase === "declined") return line("Not tagged.");
  if (state.phase === "applied") {
    const row = line(
      `Tagged “${offer.name}”.`,
      button("Undo", "ghost", () => void undo(state, record, set)),
    );
    if (state.reason !== undefined)
      row.appendChild(el("p", "refusal", state.reason));
    return row;
  }
  const row = el("div", "choices");
  const known = state.tagID !== "";
  if (!known && !offer.mayCreate) {
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
          `Tag the ${record.record_type} ${record.record_id} “${offer.name}”${known ? "" : ", adding the tag first"}.`,
        ),
      ),
    );
    return row;
  }
  const busy = state.phase === "busy";
  row.append(
    button(
      known ? "Tag it" : "Add the tag and tag it",
      "primary",
      () => void accept(offer, record, state, set),
      busy,
    ),
    button(
      "No thanks",
      "ghost",
      () => set({ ...state, phase: "declined" }),
      busy,
    ),
  );
  if (state.reason !== undefined)
    row.appendChild(el("p", "refusal", state.reason));
  return row;
}

/** accept coins the word when it is new, then applies it by id. A coined word
 *  stays coined: when the apply fails the state keeps its id, so the retry
 *  applies directly. */
async function accept(
  offer: Offer,
  record: Target,
  state: State,
  set: (next: State) => void,
): Promise<void> {
  set({ ...state, phase: "busy", reason: undefined });
  let tagID = state.tagID;
  if (tagID === "") {
    const made = await callServerTool("create_tag", { name: offer.name });
    if (!made.ok) {
      set(failure(state, made));
      return;
    }
    tagID = asText(asRecord(made.data).tag_id);
    state = { ...state, tagID };
  }
  const applied = await callServerTool("apply_tag", {
    ...record,
    tag_id: tagID,
  });
  set(
    applied.ok
      ? { phase: "applied", tagID }
      : {
          ...failure(state, applied),
          phase: applied.unknown === true ? "unknown" : "coined",
        },
  );
}

async function undo(
  state: State,
  record: Target,
  set: (next: State) => void,
): Promise<void> {
  set({ ...state, phase: "busy", reason: undefined });
  const removed = await callServerTool("remove_tag", {
    ...record,
    tag_id: state.tagID,
  });
  set(
    removed.ok
      ? { phase: "declined", tagID: state.tagID }
      : {
          ...failure(state, removed),
          phase: removed.unknown === true ? "unknown" : "applied",
        },
  );
}

function failure(
  state: State,
  result: { ok: false; reason: string; unknown?: true },
): State {
  return {
    tagID: state.tagID,
    phase: result.unknown === true ? "unknown" : "idle",
    reason: result.reason,
  };
}

function line(text: string, extra?: HTMLElement): HTMLElement {
  const row = el("div", "settled");
  row.appendChild(el("p", undefined, text));
  if (extra !== undefined) row.appendChild(extra);
  return row;
}
