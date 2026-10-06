// The pipeline review: whats_slipping_this_week's ranked deals, worst first,
// each with the evidence its risk claim rests on — drawn as the app draws a
// queue, a rank chip per row and the amount at its far end.
//
// WHY THE EVIDENCE IS ON THE ROW AND NOT BEHIND A DISCLOSURE. The rank is a
// judgement, and the tool's whole contract is that a deal whose risk cannot be
// evidenced from its own fields is absent rather than guessed. A view that
// showed the ranking and hid the reasons would present that judgement as an
// oracle — which is the one reading the tool is written to prevent.
//
// It registers no tool of its own. `render_pipeline_review` is a document hung
// off a tool that already answers, which is what every `render_*` name on this
// surface is.

import { ABSENT, count, el, money, onResult } from "../bridge";
import { panel, panelFoot, panelRow } from "../parts";
import { asList, asRecord, asText, type Warning } from "../types";
import "../view.css";

type SlippingDeal = {
  rank: string;
  name: string;
  amount: string;
  evidence: { source: string; snippet: string }[];
};

/**
 * known narrows the untrusted payload, filtering BEFORE anything is counted so
 * the head describes what is actually shown.
 */
function known(data: Record<string, unknown>): SlippingDeal[] {
  return asList(data.deals)
    .filter(
      (d): d is Record<string, unknown> => typeof d === "object" && d !== null,
    )
    .map((d) => ({
      // The tool's OWN rank, not this array's index. They agree today and the
      // difference only shows when a payload carries a row this view drops —
      // at which point renumbering would put a rank on screen the tool never
      // answered.
      rank: count(d.rank),
      name: asText(d.name) || asText(d.deal_id),
      // Absent, not zero. A deal can be worked before it is priced, and a
      // blank amount rendered as a currency zero says it is worth nothing.
      amount: money(d.amount_minor, d.currency),
      evidence: asList(d.evidence).map(evidenceOf),
    }));
}

function evidenceOf(entry: unknown): { source: string; snippet: string } {
  const evidence = asRecord(entry);
  return { source: asText(evidence.source), snippet: asText(evidence.snippet) };
}

/** evidenceLine is one reason, followed by the field it was read off: the
 *  field is the proof, so it is shown rather than summarized away. */
function evidenceLine(evidence: {
  source: string;
  snippet: string;
}): HTMLElement {
  const line = el("div", "meta evidence", evidence.snippet);
  if (evidence.source !== "") {
    line.append(" · ", el("span", "source", evidence.source));
  }
  return line;
}

function dealRow(deal: SlippingDeal): HTMLElement {
  const row = panelRow("item");
  row.appendChild(el("span", "rank", deal.rank));
  const main = el("div");
  main.appendChild(el("div", "name", deal.name));
  if (deal.evidence.length === 0) {
    // The tool does not answer an unevidenced deal, so this is a payload that
    // did not come from it. Saying so beats rendering a rank with no reason.
    main.appendChild(
      el("div", "meta evidence", "no evidence was sent for this deal"),
    );
  }
  for (const evidence of deal.evidence)
    main.appendChild(evidenceLine(evidence));
  row.appendChild(main);
  row.appendChild(
    el(
      "span",
      deal.amount === ABSENT ? "figure figure-absent" : "figure",
      deal.amount,
    ),
  );
  return row;
}

export function render(
  root: HTMLElement,
  data: unknown,
  _warnings: Warning[],
): void {
  root.replaceChildren();
  if (data === null || data === undefined) {
    root.appendChild(
      el(
        "div",
        "empty empty-alone",
        "The host sent no structured result for this review.",
      ),
    );
    return;
  }
  const answer = asRecord(data);
  // The member has to BE an array, for the reason the commitments view gives:
  // an absent one renders as "no deal's risk can be evidenced", which is a
  // definite answer about the pipeline rather than an admission that the
  // payload could not be read.
  if (!Array.isArray(answer.deals)) {
    root.appendChild(
      el(
        "div",
        "empty empty-alone",
        "The host sent no readable pipeline review.",
      ),
    );
    return;
  }
  const deals = known(answer);
  // "shown", not "at risk": the caller may have asked for a capped set, and
  // this document cannot tell a top-five from the whole answer. The number
  // describes the panel, which is a claim it can keep.
  const review = panel("Slipping this week", {
    level: "h1",
    action: el("span", "meta", `${deals.length} shown`),
  });
  if (deals.length === 0) {
    review.appendChild(
      el(
        "p",
        "empty",
        "No deal's risk can be evidenced from its own fields. " +
          "That is the answer, not a gap.",
      ),
    );
    root.appendChild(review);
    return;
  }
  for (const deal of deals) review.appendChild(dealRow(deal));
  const foot = panelFoot();
  // No count here: the head already says how many are SHOWN, and a count in the
  // foot would read as how many are at risk, which a capped answer cannot say.
  foot.appendChild(
    el("p", "meta", "Worst first. Each risk names the field it was read from."),
  );
  review.appendChild(foot);
  root.appendChild(review);
}

onResult((data, warnings) => {
  const root = document.getElementById("root");
  // Guarded rather than asserted; see the account brief for why.
  if (root !== null) render(root, data, warnings);
});
