// The story-so-far card: what read_context answered. A meeting leads with
// what the meeting is for, then the plan a reader takes into the room. A
// catch-up lists what happened on one record, newest first in each part. This
// morning's brief counts the deals still to look at.
//
// Each card's last line puts a follow-up question in the chat. The card calls
// no tool itself.

import { askAssistant } from "../actions";
import { badge } from "../badge";
import { el } from "../bridge";
import { hero, type Part, ring } from "../hero";
import { eyebrow, panel, panelBody, rowAction, viewRenderer } from "../parts";
import { refusalPanel } from "../summary";
import {
  asFiniteNumber,
  asList,
  asRecord,
  asText,
  type ToolCall,
  type Warning,
} from "../types";
import { ABSENT, day } from "../values";
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
  if (call.refusal !== null) {
    root.appendChild(refusalPanel("Nothing to show", call.refusal));
    return;
  }
  const answer = asRecord(data);
  if ("briefing" in answer) {
    root.appendChild(meetingCard(answer));
  } else if (Array.isArray(answer.sections)) {
    root.appendChild(catchUpCard(answer));
  } else if (Array.isArray(answer.items)) {
    root.appendChild(briefCard(answer));
  } else {
    root.appendChild(refusalPanel("Nothing to show", null));
  }
}

/** A titled part of a card: the small title and the lines under it. */
function part(title: string, lines: readonly HTMLElement[]): HTMLElement {
  const block = el("section", "context-part");
  const list = el("ul", "context-lines");
  list.append(...lines);
  block.append(eyebrow(title), list);
  return block;
}

/** A line of a part: what it says, and a quieter note under it. */
function line(text: string, note = ""): HTMLElement {
  const item = el("li", "context-line");
  item.appendChild(el("p", "context-line-text", text));
  if (note !== "") item.appendChild(el("p", "meta", note));
  return item;
}

/** textOf reads a grounded sentence: an object whose text is the words. */
function textOf(value: unknown): string {
  return asText(asRecord(value).text);
}

function meetingCard(answer: Record<string, unknown>): HTMLElement {
  const plan = asRecord(asRecord(answer.brief).plan);
  const prepared = asText(plan.readiness) === "prepared";
  const card = panel("Before the meeting", {
    level: "h1",
    ...("readiness" in plan
      ? {
          action: badge(
            prepared ? "Prepared" : "Outline",
            prepared ? "success" : "default",
          ),
        }
      : {}),
  });
  const body = panelBody();
  const objective = textOf(plan.objective);
  body.appendChild(
    el(
      "p",
      "intro",
      objective === "" ? "What to know before you go in." : objective,
    ),
  );
  const opening = textOf(plan.opening);
  if (opening !== "") body.appendChild(part("Open with", [line(opening)]));
  const questions = asList(plan.questions)
    .map(asRecord)
    .filter((q) => asText(q.ask) !== "")
    .map((q) => line(asText(q.ask), asText(q.why)));
  if (questions.length > 0) body.appendChild(part("Ask", questions));
  const asks = asList(plan.likely_asks)
    .map(asRecord)
    .filter((a) => asText(a.question) !== "")
    .map((a) => line(asText(a.question), asText(a.prepare)));
  if (asks.length > 0) body.appendChild(part("They may ask", asks));
  const risk = asRecord(plan.top_risk);
  if (asText(risk.say) !== "") {
    const avoid = asText(risk.avoid);
    body.appendChild(
      part("The risk", [
        line(
          textOf(risk.text) || asText(risk.say),
          avoid === "" ? "" : `Avoid: ${avoid}`,
        ),
      ]),
    );
  }
  const focus = asList(answer.meeting_focus)
    .map((f) => asText(asRecord(f).summary))
    .filter((s) => s !== "")
    .map((s) => line(s));
  if (focus.length > 0) body.appendChild(part("Still open", focus));
  const recent = sectionLines(asRecord(answer.briefing), "recent_touches");
  if (recent.length > 0) body.appendChild(part("Lately", recent.slice(0, 3)));
  body.appendChild(
    rowAction("Draft my opening", () =>
      askAssistant(
        "Draft what I should say in the first two minutes of this meeting, in my own voice, from the brief you just read.",
      ),
    ),
  );
  card.appendChild(body);
  return card;
}

/** sectionLines are one named section's items, each with its day. */
function sectionLines(
  context: Record<string, unknown>,
  name: string,
): HTMLElement[] {
  const section = asList(context.sections)
    .map(asRecord)
    .find((s) => asText(s.name) === name);
  return section === undefined ? [] : itemLines(section);
}

function itemLines(section: Record<string, unknown>): HTMLElement[] {
  return asList(section.items)
    .map(asRecord)
    .filter((item) => asText(item.summary) !== "")
    .map((item) => {
      const when = day(item.occurred_at);
      return line(asText(item.summary), when === ABSENT ? "" : when);
    });
}

/** sectionTitle names a part of the story as a reader would. */
function sectionTitle(name: string): string {
  switch (name) {
    case "recent_touches":
      return "Lately";
    case "open_tasks":
      return "Still to do";
    case "who_knows":
      return "Who knows them";
    case "also_present":
      return "Also there";
    case "unresolved_attendees":
      return "Not yet in the CRM";
    default:
      return name.startsWith("related_")
        ? `Related ${name.slice("related_".length).replaceAll("_", " ")}`
        : "More";
  }
}

/** anchorNoun names the kind of record the story is about. */
function anchorNoun(recordType: string): string {
  switch (recordType) {
    case "contact":
    case "company":
    case "deal":
    case "lead":
    case "project":
      return recordType;
    default:
      return "record";
  }
}

function catchUpCard(answer: Record<string, unknown>): HTMLElement {
  const noun = anchorNoun(asText(asRecord(answer.anchor).record_type));
  const sections = asList(answer.sections).map(asRecord);
  const count = (name: string) =>
    asList(sections.find((s) => asText(s.name) === name)?.items).length;
  const card = panel("The story so far", { level: "h1", action: badge(noun) });
  const body = panelBody();
  const touches = count("recent_touches");
  const tasks = count("open_tasks");
  body.appendChild(
    hero({
      figure: String(touches),
      caption: touches === 1 ? "recent touch" : "recent touches",
      sentence:
        tasks === 0
          ? `Nothing is still to do on this ${noun}.`
          : `${tasks} ${tasks === 1 ? "thing is" : "things are"} still to do on this ${noun}.`,
    }),
  );
  for (const section of sections) {
    const lines = itemLines(section);
    if (lines.length > 0) {
      body.appendChild(part(sectionTitle(asText(section.name)), lines));
    }
  }
  body.appendChild(
    rowAction("What should I do next", () =>
      askAssistant(
        `From what you just read about this ${noun}, what is the one thing I should do next, and why?`,
      ),
    ),
  );
  card.appendChild(body);
  return card;
}

function briefCard(answer: Record<string, unknown>): HTMLElement {
  const items = asList(answer.items).map(asRecord);
  const count = (state: string) =>
    items.filter((i) => asText(i.state) === state).length;
  const fresh = count("new");
  const card = panel("This morning's brief", {
    level: "h1",
    action: badge(day(answer.local_day)),
  });
  const body = panelBody();
  if (items.length === 0) {
    body.appendChild(el("p", "intro", "Nothing is on the brief today."));
    card.appendChild(body);
    return card;
  }
  const parts: Part[] = [
    { label: "To look at", value: fresh, tone: "accent" },
    { label: "Done", value: count("acted"), tone: "success" },
    { label: "Snoozed", value: count("snoozed"), tone: "neutral" },
    { label: "Dismissed", value: count("dismissed"), tone: "neutral" },
  ];
  const candidates = asFiniteNumber(answer.candidate_count);
  body.appendChild(
    hero({
      figure: String(fresh),
      caption: fresh === 1 ? "deal to look at" : "deals to look at",
      sentence:
        candidates === null
          ? "Ranked by what each deal is worth and how soon it needs you."
          : `Picked from ${candidates} open ${candidates === 1 ? "deal" : "deals"}, ranked by what each is worth and how soon it needs you.`,
      beside: ring(parts, String(items.length), "on the brief"),
    }),
  );
  body.appendChild(
    rowAction("Walk me through it", () =>
      askAssistant(
        "Walk me through this morning's brief, top deal first: what each one needs from me today, with its link.",
      ),
    ),
  );
  card.appendChild(body);
  return card;
}
