import { ArrowDown, ArrowUp, X } from "lucide-react";
import { useState } from "react";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { IconAction } from "../../design-system/iconaction";
import { Select } from "../../design-system/select";
import { type Translator, useT } from "../../i18n";
import {
  type CardChoice,
  layoutEdits as edit,
  MAX_SECTIONS,
  type ReportingLayout,
} from "./reporting.layout";
import { blockLabel, metricLabel } from "./reporting.model";

type Card = ReportingLayout[number]["cards"][number];

/** A card or a choice by its chart's name, and its metric's when it has one. */
export function cardLabel(
  card: Pick<Card, "kind" | "metric"> | CardChoice,
  t: Translator,
): string {
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
}: Readonly<{
  layout: ReportingLayout;
  choices: readonly CardChoice[];
  onChange: (next: ReportingLayout) => void;
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
          <legend>{t("reporting.sectionN", { n: String(at + 1) })}</legend>
          <Field label={t("reporting.sectionTitle", { n: String(at + 1) })}>
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
                {t("reporting.removeSection", { n: String(at + 1) })}
              </Button>
            </div>
          )}
        </fieldset>
      ))}
      <AddCard layout={layout} choices={choices} onChange={onChange} />
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
          value={String(at)}
          options={layout.map((_, target) => ({
            value: String(target),
            label: t("reporting.sectionN", { n: String(target + 1) }),
          }))}
          onChange={(value) =>
            onChange(edit.moveCardTo(layout, at, index, Number(value)))
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
}: Readonly<{
  layout: ReportingLayout;
  choices: readonly CardChoice[];
  onChange: (next: ReportingLayout) => void;
}>) {
  const t = useT();
  const [picked, setPicked] = useState("");
  const keyOf = (choice: CardChoice) => `${choice.kind}:${choice.metric ?? ""}`;
  const choice = choices.find((candidate) => keyOf(candidate) === picked);
  if (choices.length === 0) return null;
  return (
    <div className="reporting-layout-add">
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
      <Button
        disabled={!choice}
        onClick={() => {
          if (!choice) return;
          onChange(edit.addCard(layout, choice));
          setPicked("");
        }}
      >
        {t("reporting.addChart")}
      </Button>
    </div>
  );
}
