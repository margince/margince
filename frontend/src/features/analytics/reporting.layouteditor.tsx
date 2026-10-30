import { ArrowDown, ArrowUp, X } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { IconAction } from "../../design-system/iconaction";
import { Select } from "../../design-system/select";
import { SortableList } from "../../design-system/sortablelist";
import { formatNumber, ordinalNumber } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import { entityLabel } from "./analytics.questions.vocab";
import {
  type CardChoice,
  layoutEdits as edit,
  MAX_SECTIONS,
  type ReportingLayout,
} from "./reporting.layout";
import { blockLabel, type MetricNames } from "./reporting.model";
import { useMetricNames } from "./reporting.queries";
import { AddQuestion } from "./reporting.questionpicker";

type Card = ReportingLayout[number]["cards"][number];

/** A card or a choice by its chart's name, and its metric's when it has one. */
export function cardLabel(
  card: Pick<Card, "kind" | "metric" | "question"> | CardChoice,
  t: Translator,
  metricName: MetricNames,
): string {
  if (card.kind === "question")
    return t("reporting.questionCard", {
      population: entityLabel(
        t,
        ("question" in card && card.question?.entity) || "",
      ),
    });
  if (card.metric && card.kind === card.metric) return metricName(card.metric);
  const chart = blockLabel(card.kind, t);
  return card.metric ? `${chart} · ${metricName(card.metric)}` : chart;
}

/**
 * Sections of chart cards. A card is dragged by its handle or moved with the
 * arrow keys on it. A menu moves it to another section, and buttons move a
 * section. So a keyboard and a screen reader can do every step.
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
  const metricName = useMetricNames();
  const { locale } = useLocale();
  const hintId = useId();
  const sections = useSectionKeys(layout.length);
  const place = (position: number, count: number) => ({
    position: formatNumber(position, locale),
    total: formatNumber(count, locale),
  });
  return (
    <div className="form-stack">
      <span id={hintId} className="sr-only">
        {t("reporting.cardOrderHint")}
      </span>
      {layout.map((section, at) => (
        <fieldset
          key={sections.keys[at]}
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
          <SortableList
            label={t("reporting.sectionN", { n: ordinalNumber(at + 1) })}
            items={section.cards.map((card) => ({ key: card.id, card }))}
            hintId={hintId}
            onReorder={(keys) => onChange(edit.reorderCards(layout, at, keys))}
            labels={{
              handle: (item, position, count) =>
                t("reporting.cardHandle", {
                  chart: cardLabel(item.card, t, metricName),
                  ...place(position, count),
                }),
              moved: (item, position, count) =>
                t("reporting.cardMoved", {
                  chart: cardLabel(item.card, t, metricName),
                  ...place(position, count),
                }),
            }}
            renderItem={(item) => (
              <CardRow
                layout={layout}
                at={at}
                index={section.cards.indexOf(item.card)}
                card={item.card}
                onChange={onChange}
              />
            )}
          />
          {layout.length > 1 && (
            <SectionActions
              layout={layout}
              at={at}
              onMove={(step) => {
                sections.move(at, step);
                onChange(edit.moveSection(layout, at, step));
              }}
              onRemove={() => {
                sections.remove(at);
                onChange(edit.removeSection(layout, at));
              }}
            />
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
          onClick={() => {
            sections.add();
            onChange(edit.addSection(layout));
          }}
        >
          {t("reporting.addSection")}
        </Button>
      </div>
    </div>
  );
}

/**
 * A key for each section while the editor is open. A section has no id, and a
 * key by position would hand a moved section's focus and drag to the one that
 * took its place. Keys move with their section; a layout of another length
 * from outside gets fresh ones.
 */
function useSectionKeys(count: number) {
  const fresh = (minted: number, length: number) => ({
    keys: Array.from({ length }, (_, index) => `section-key-${minted + index}`),
    minted: minted + length,
  });
  const [state, setState] = useState(() => fresh(0, count));
  const current =
    state.keys.length === count ? state : fresh(state.minted, count);
  if (current !== state) setState(current);
  const keep = (keys: string[]) => setState({ ...current, keys });
  return {
    keys: current.keys,
    move: (at: number, step: -1 | 1) => {
      const next = [...current.keys];
      const [moving] = next.splice(at, 1);
      if (moving) next.splice(at + step, 0, moving);
      keep(next);
    },
    remove: (at: number) =>
      keep(current.keys.filter((_, index) => index !== at)),
    add: () =>
      setState({
        keys: [...current.keys, `section-key-${current.minted}`],
        minted: current.minted + 1,
      }),
  };
}

// Moves a section up or down, or removes it and keeps its cards.
function SectionActions({
  layout,
  at,
  onMove,
  onRemove,
}: Readonly<{
  layout: ReportingLayout;
  at: number;
  onMove: (step: -1 | 1) => void;
  onRemove: () => void;
}>) {
  const t = useT();
  const name = t("reporting.sectionN", { n: ordinalNumber(at + 1) });
  return (
    <div className="reporting-layout-buttons">
      <IconAction
        label={`${t("reporting.moveUp")}: ${name}`}
        icon={<ArrowUp aria-hidden />}
        disabled={at === 0}
        onClick={() => onMove(-1)}
      />
      <IconAction
        label={`${t("reporting.moveDown")}: ${name}`}
        icon={<ArrowDown aria-hidden />}
        disabled={at === layout.length - 1}
        onClick={() => onMove(1)}
      />
      <Button variant="ghost" onClick={onRemove}>
        {t("reporting.removeSection", { n: ordinalNumber(at + 1) })}
      </Button>
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
  const metricName = useMetricNames();
  const name = cardLabel(card, t, metricName);
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
  const metricName = useMetricNames();
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
              label: cardLabel(candidate, t, metricName),
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
