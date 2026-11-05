// The update card: what an update_record call changed, field by field, or why it
// changed nothing. Every update reaches this card. It calls no tool, so it
// declares no actions.

import { el } from "../bridge";
import { viewRenderer } from "../parts";
import {
  changesOf,
  changeTable,
  recordLine,
  recordName,
  recordNoun,
  refusalPanel,
  savedPanel,
  withArticle,
} from "../summary";
import { asRecord, asText, type ToolCall, type Warning } from "../types";
import "../view.css";

/** render draws the card, keeping the reader's place across a redraw. */
export const render = viewRenderer(draw);

function draw(
  root: HTMLElement,
  data: unknown,
  _warnings: Warning[],
  call: ToolCall,
): void {
  root.replaceChildren();
  root.appendChild(
    call.refusal === null
      ? updatedPanel(asRecord(data))
      : refusalPanel("Nothing was changed", call.refusal),
  );
}

/** updatedPanel says what an update changed. */
function updatedPanel(updated: Record<string, unknown>): HTMLElement {
  const fields = asRecord(updated.fields);
  const noun = recordNoun(asText(updated.record_type), fields);
  const name = recordName(fields);
  const rows = changeTable(changesOf(updated.changes), { before: true });
  const said = rows ?? el("p", "intro", "The change is saved in Margince.");
  if (name === "") return savedPanel(`Updated ${withArticle(noun)}`, said);
  return savedPanel(
    `Updated ${withArticle(noun)}`,
    recordLine(name, asText(updated.id), noun),
    said,
  );
}
