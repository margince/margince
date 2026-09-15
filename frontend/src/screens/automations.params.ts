// The parameter schema an automation template carries, read into the fields a
// form can draw. It is JSON Schema shaped by the catalogue rather than by this
// screen, so it lives apart from the screen that renders the result: the
// reading is testable on its own, and the form file stays about the form.

export type ParamField = {
  key: string;
  kind: "integer" | "string" | "boolean" | "date_field" | "enum";
  min?: number;
  max?: number;
  initial: string;
  // Set only for kind "enum" — the schema's own closed value list (e.g.
  // renewal_reminder's object property), rendered as a picker instead of
  // a free-text box so a typo can't silently name a value the backend
  // would refuse.
  options?: string[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// Catalog defaults and stored params are JSON scalars; anything non-scalar
// has no honest single-line rendering, so it collapses to empty.
export function scalarText(value: unknown): string {
  if (value === undefined || value === null || typeof value === "object") {
    return "";
  }
  return String(value);
}

function paramKind(type: unknown): ParamField["kind"] | null {
  if (type === "integer" || type === "number") {
    return "integer";
  }
  if (type === "boolean") {
    return "boolean";
  }
  if (type === "string") {
    return "string";
  }
  return null;
}

// enumOptions reads a schema property's own closed value list, when it
// has one — a string-typed "enum" array is the schema's way of saying
// "pick one of these", which renders as a picker rather than free text
// so a typo can't silently name a value the backend would refuse.
function enumOptions(raw: Record<string, unknown>): string[] | undefined {
  if (!Array.isArray(raw.enum)) {
    return undefined;
  }
  const values = raw.enum.filter((v): v is string => typeof v === "string");
  return values.length > 0 ? values : undefined;
}

// The ONLY source of editable parameters: the catalog entry's JSON schema.
export function paramFields(schema: Record<string, unknown>): ParamField[] {
  const properties = isRecord(schema.properties) ? schema.properties : {};
  // renewal_reminder's date_field names a workspace's own cf_* column, not a
  // fixed value — but nothing in the JSON schema type system marks a string
  // as "this is a column reference". The schema DOES say which object owns
  // it (its sibling `object` property), and only a schema declaring BOTH has
  // enough context to resolve a column list, so that pairing — not the bare
  // key name, which some future automation could reuse for something
  // unrelated — is what selects the picker over a free-text box.
  const isDateFieldPicker =
    "object" in properties && "date_field" in properties;
  return Object.entries(properties).flatMap(([key, raw]) => {
    if (!isRecord(raw)) {
      return [];
    }
    const options = enumOptions(raw);
    const kind: ParamField["kind"] | null =
      key === "date_field" && isDateFieldPicker
        ? "date_field"
        : options
          ? "enum"
          : paramKind(raw.type);
    if (kind === null) {
      return [];
    }
    return [
      {
        key,
        kind,
        min: typeof raw.minimum === "number" ? raw.minimum : undefined,
        max: typeof raw.maximum === "number" ? raw.maximum : undefined,
        initial: scalarText(raw.default),
        options,
      },
    ];
  });
}

export function paramsFromValues(
  fields: ParamField[],
  values: Record<string, string>,
): Record<string, unknown> {
  return Object.fromEntries(
    fields.map((field) => {
      const value = values[field.key] ?? field.initial;
      if (field.kind === "integer") {
        return [field.key, Number(value)];
      }
      if (field.kind === "boolean") {
        return [field.key, value === "true"];
      }
      return [field.key, value];
    }),
  );
}

// One schema-derived param's control, lifted out of AutomationForm so that
// function stays a shape a reader can hold at once as this grows a third
// input kind. A Checkbox carries its own label (the design system's own
// Checkbox/Switch split: this STATES an intent the Save button below then
// submits, so the field's usual heading span would just repeat it) — every
// other kind keeps the heading span the rest of the form uses.
