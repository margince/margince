// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Card } from "../design-system/atoms";
import { PanelBody, PanelGroupHead } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { WeeklyReview } from "./brief.queries";
import { EntityRef } from "./entityref";

// The interval between two lessons is declared beside the week's own, in the
// sheet the panel that holds this one imports.
import "./brief.weekly.css";

// What the week taught, beside what it was.
//
// A GROUP inside the week's panel rather than a panel of its own: a titled
// boxed surface inside another panel's column reads as a second product, and
// the pane already has one head.
//
// EVERY LEARNING SHOWS WHAT IT RESTS ON. The narrative above this describes a
// week the reader can already see; a learning is a claim about cause that they
// cannot check against anything on the page. The citations are what make it
// checkable, so they are drawn beside the claim rather than folded away — a
// lesson whose sources are one click out of sight is a lesson read as fact.

type Learnings = NonNullable<WeeklyReview["learnings"]>;
type Learning = Learnings["items"][number];

// The four shapes, and the label each carries. A closed map rather than a
// formatted key: a kind the server adds without a label here would otherwise
// reach a reader as a raw enum value.
const KIND_LABEL: Readonly<Record<Learning["kind"], MessageKey>> = {
  worked: "brief.weekly.learnings.worked",
  did_not_work: "brief.weekly.learnings.didNotWork",
  pattern: "brief.weekly.learnings.pattern",
  experiment: "brief.weekly.learnings.experiment",
};

export function LearningsPanel({
  learnings,
}: Readonly<{ learnings: Learnings | undefined }>) {
  const t = useT();
  // A review written before this lane existed carries nothing at all. Drawing
  // an empty panel would state a verdict about a week nobody looked at.
  if (!learnings) return null;

  return (
    <>
      <PanelGroupHead title={t("brief.weekly.learnings.title")} level="h3" />
      <PanelBody className="brief-weekly-learnings">
        {learnings.items.length === 0 ? (
          <EmptyLearnings state={learnings.state} />
        ) : (
          learnings.items.map((item) => (
            <LearningCard
              // Keyed on what the learning RESTS ON rather than its position:
              // a frozen week's list never reorders, but a key built from an
              // index says the opposite to anybody reading it later. The first
              // citation is present on every learning — one that cites nothing
              // is refused before it is stored.
              key={`${item.citations[0].subject_id}-${item.kind}`}
              learning={item}
            />
          ))
        )}
      </PanelBody>
    </>
  );
}

// NOBODY LOOKED and FOUND NOTHING are different weeks.
//
// The state column exists for exactly this sentence. A rep whose lane was
// unbound, whose budget ran out or whose provider was down has not been told
// their week held no lesson — nothing has read it — and saying otherwise is a
// claim the product has not earned.
function EmptyLearnings({ state }: Readonly<{ state: Learnings["state"] }>) {
  const t = useT();
  return (
    <p className="weekly-learnings-empty">
      {state === "not_run"
        ? t("brief.weekly.learnings.notRun")
        : t("brief.weekly.learnings.insufficient")}
    </p>
  );
}

function LearningCard({ learning }: Readonly<{ learning: Learning }>) {
  const t = useT();
  return (
    <Card title={t(KIND_LABEL[learning.kind])}>
      <p>{learning.text}</p>
      <ul className="weekly-learnings-citations">
        {learning.citations.map((c) => (
          <li key={`${c.subject_type}-${c.subject_id}`}>
            {/* A deal links through to its record; a commitment is a row of the
                rep's own plan with no 360 to send them to, so it is named and
                not linked.

                Either way the label is the one FROZEN into the week, passed so
                the link does not resolve today's name — a deal renamed in March
                must not relabel the lesson it taught in January. */}
            {c.subject_type === "deal" ? (
              <EntityRef kind="deal" id={c.subject_id} name={c.label} />
            ) : (
              c.label
            )}
          </li>
        ))}
      </ul>
    </Card>
  );
}
