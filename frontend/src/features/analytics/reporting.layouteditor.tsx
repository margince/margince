import { useMutation } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, X } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { IconAction } from "../../design-system/iconaction";
import { Select } from "../../design-system/select";
import { ordinalNumber } from "../../format/format";
import { type Translator, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { entityLabel } from "./analytics.questions.vocab";
import {
  type CardChoice,
  cardQuestion,
  layoutEdits as edit,
  MAX_CARDS,
  MAX_SECTIONS,
  type ReportingLayout,
  runIdOf,
} from "./reporting.layout";
import { blockLabel, metricLabel } from "./reporting.model";

type Card = ReportingLayout[number]["cards"][number];

/** A card or a choice by its chart's name, and its metric's when it has one. */
export function cardLabel(
  card: Pick<Card, "kind" | "metric" | "question"> | CardChoice,
  t: Translator,
): string {
  if (card.kind === "question")
    return t("reporting.questionCard", {
      population: entityLabel(
        t,
        ("question" in card && card.question?.entity) || "",
      ),
    });
  const chart = blockLabel(card.kind, t);
  return card.metric ? `${chart} · ${metricLabel(card.metric, t)}` : chart;
}

/**
 * Sections of chart cards, edited with buttons and menus rather than by
 * dragging, so a keyboard and a screen reader can do every step.
 */
export function LayoutEditor({
  layout,
  choices,
  onChange,
  preview,
}: Readonly<{
  layout: ReportingLayout;
  choices: readonly CardChoice[];
  onChange: (next: ReportingLayout) => void;
  // Draws one chart before it is added; absent offers no preview.
  preview?: (choice: CardChoice) => ReactNode;
}>) {
  const t = useT();
  return (
    <div className="form-stack">
      {layout.map((section, at) => (
        <fieldset
          // biome-ignore lint/suspicious/noArrayIndexKey: a section has no id; its position is its name
          key={at}
          className="form-stack reporting-layout-section"
        >
          <legend>
            {t("reporting.sectionN", { n: ordinalNumber(at + 1) })}
          </legend>
          <Field
            label={t("reporting.sectionTitle", { n: ordinalNumber(at + 1) })}
          >
            {(field) => (
              <TextInput
                {...field}
                value={section.title ?? ""}
                maxLength={120}
                onChange={(event) =>
                  onChange(edit.renameSection(layout, at, event.target.value))
                }
              />
            )}
          </Field>
          {section.cards.map((card, index) => (
            <CardRow
              key={card.id}
              layout={layout}
              at={at}
              index={index}
              card={card}
              onChange={onChange}
            />
          ))}
          {layout.length > 1 && (
            <div>
              <Button
                variant="ghost"
                onClick={() => onChange(edit.removeSection(layout, at))}
              >
                {t("reporting.removeSection", { n: ordinalNumber(at + 1) })}
              </Button>
            </div>
          )}
        </fieldset>
      ))}
      <AddCard
        layout={layout}
        choices={choices}
        onChange={onChange}
        preview={preview}
      />
      <AddQuestion layout={layout} onChange={onChange} />
      <div>
        <Button
          variant="ghost"
          disabled={layout.length >= MAX_SECTIONS}
          onClick={() => onChange(edit.addSection(layout))}
        >
          {t("reporting.addSection")}
        </Button>
      </div>
    </div>
  );
}

// A section option's value names the section to the menu and is never shown.
const sectionKey = (at: number) => `section-${at}`;

function CardRow({
  layout,
  at,
  index,
  card,
  onChange,
}: Readonly<{
  layout: ReportingLayout;
  at: number;
  index: number;
  card: Card;
  onChange: (next: ReportingLayout) => void;
}>) {
  const t = useT();
  const name = cardLabel(card, t);
  const count = layout[at]?.cards.length ?? 0;
  return (
    <div className="reporting-layout-card">
      <span className="reporting-layout-name">{name}</span>
      <Select
        aria-label={t("reporting.cardWidth", { chart: name })}
        value={card.width}
        options={[
          { value: "full", label: t("reporting.widthFull") },
          { value: "half", label: t("reporting.widthHalf") },
        ]}
        onChange={(value) => {
          if (value === "full" || value === "half")
            onChange(edit.setWidth(layout, at, index, value));
        }}
      />
      {layout.length > 1 && (
        <Select
          aria-label={t("reporting.cardSection", { chart: name })}
          value={sectionKey(at)}
          options={layout.map((_, target) => ({
            value: sectionKey(target),
            label: t("reporting.sectionN", { n: ordinalNumber(target + 1) }),
          }))}
          onChange={(value) =>
            onChange(
              edit.moveCardTo(
                layout,
                at,
                index,
                Number(value.replace("section-", "")),
              ),
            )
          }
        />
      )}
      <IconAction
        label={`${t("reporting.moveUp")}: ${name}`}
        icon={<ArrowUp aria-hidden />}
        disabled={index === 0}
        onClick={() => onChange(edit.moveCard(layout, at, index, -1))}
      />
      <IconAction
        label={`${t("reporting.moveDown")}: ${name}`}
        icon={<ArrowDown aria-hidden />}
        disabled={index === count - 1}
        onClick={() => onChange(edit.moveCard(layout, at, index, 1))}
      />
      <IconAction
        label={t("reporting.removeCard", { chart: name })}
        icon={<X aria-hidden />}
        onClick={() => onChange(edit.removeCard(layout, at, index))}
      />
    </div>
  );
}

function AddCard({
  layout,
  choices,
  onChange,
  preview,
}: Readonly<{
  layout: ReportingLayout;
  choices: readonly CardChoice[];
  onChange: (next: ReportingLayout) => void;
  preview?: (choice: CardChoice) => ReactNode;
}>) {
  const t = useT();
  const [picked, setPicked] = useState("");
  const [previewing, setPreviewing] = useState(false);
  const keyOf = (choice: CardChoice) => `${choice.kind}:${choice.metric ?? ""}`;
  const choice = choices.find((candidate) => keyOf(candidate) === picked);
  if (choices.length === 0) return null;
  return (
    <div className="form-stack">
      <Field label={t("reporting.chooseChart")}>
        {(field) => (
          <Select
            {...field}
            value={choice ? picked : ""}
            placeholder={t("reporting.chooseChart")}
            options={choices.map((candidate) => ({
              value: keyOf(candidate),
              label: cardLabel(candidate, t),
            }))}
            onChange={setPicked}
          />
        )}
      </Field>
      <div className="reporting-layout-buttons">
        {preview && (
          <Button
            variant="ghost"
            disabled={!choice}
            aria-expanded={previewing && !!choice}
            onClick={() => setPreviewing(!previewing)}
          >
            {t("reporting.previewChart")}
          </Button>
        )}
        <Button
          disabled={!choice}
          onClick={() => {
            if (!choice) return;
            onChange(edit.addCard(layout, choice));
            setPicked("");
            setPreviewing(false);
          }}
        >
          {t("reporting.addChart")}
        </Button>
      </div>
      {preview && previewing && choice && preview(choice)}
    </div>
  );
}

/**
 * Adds a saved custom question from its link. The card keeps a copy of the
 * question, so the link's later edits do not move a published report.
 */
function AddQuestion({
  layout,
  onChange,
}: Readonly<{
  layout: ReportingLayout;
  onChange: (next: ReportingLayout) => void;
}>) {
  const t = useT();
  const [link, setLink] = useState("");
  // A filtered question cannot ride a report: every reader receives the
  // question, and a filter value can name a record they may not open.
  const [filtered, setFiltered] = useState(false);
  const runId = runIdOf(link);
  const read = useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.GET("/analytics/runs/{run_id}", {
          params: { path: { run_id: id } },
        }),
      ),
    onSuccess: (run) => {
      if (run.query.filters?.length) {
        setFiltered(true);
        return;
      }
      onChange(edit.addQuestion(layout, cardQuestion(run.query)));
      setLink("");
    },
  });
  if (layout.flatMap((section) => section.cards).length >= MAX_CARDS)
    return null;
  return (
    <div className="form-stack">
      <Field
        label={t("reporting.questionLink")}
        hint={link && !runId ? t("reporting.questionLinkInvalid") : undefined}
      >
        {(field) => (
          <TextInput
            {...field}
            value={link}
            onChange={(event) => {
              setLink(event.target.value);
              setFiltered(false);
            }}
          />
        )}
      </Field>
      <ErrorLine error={read.error} />
      {filtered && <p role="alert">{t("reporting.questionFiltered")}</p>}
      <div>
        <Button
          disabled={!runId}
          pending={read.isPending}
          onClick={() => runId && read.mutate(runId)}
        >
          {t("reporting.addQuestion")}
        </Button>
      </div>
    </div>
  );
}
