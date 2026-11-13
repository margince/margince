import { readdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { createServer } from "vite";
import { parseSource } from "./lib/source-tree.ts";
import { stable } from "./lib/stable.mjs";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const names = [
  "lib/approvalpreview.ts",
  "lib/approvalpreviewfields.ts",
  "lib/approvalpreviewimport.ts",
  "lib/approvalpreviewmergefields.ts",
  "lib/approvalpreviewmergevalues.ts",
  "lib/historyfieldlabels.ts",
  "lib/compose-context.ts",
  "lib/memberretirement.ts",
];
const inputs = [...names];
for (const name of await readdir(join(root, "src/format"))) {
  if (name.endsWith(".ts") && !name.includes(".test."))
    inputs.push(`format/${name}`);
}
inputs.push("i18n/en.ts", "i18n/de.ts", "i18n/vi.ts");
function addCodes(source, name) {
  const tree = parseSource(name, source);
  const visit = (node) => {
    if (ts.isStringLiteral(node)) values.add(node.text);
    if (ts.isPropertyAssignment(node) && ts.isIdentifier(node.name))
      values.add(node.name.text);
    ts.forEachChild(node, visit);
  };
  visit(tree);
}
const fields = new Set();
const values = new Set();
for (const name of inputs.sort(stable)) {
  const source = await readFile(join(root, "src", name), "utf8");
  if (!name.startsWith("i18n/")) addCodes(source, name);
  if (name.includes("preview") || name.includes("historyfieldlabels")) {
    for (const match of source.matchAll(
      /(?:^\s*([a-z][a-z0-9_]*):|"([a-z][a-z0-9_]*)")/gm,
    ))
      fields.add(match[1] ?? match[2]);
  }
}
const server = await createServer({
  root,
  server: { middlewareMode: true, hmr: false },
  appType: "custom",
});
try {
  const { en } = await server.ssrLoadModule("/src/i18n/en.ts");
  const bootstrap = Object.fromEntries(
    ["app.errorTitle", "app.errorBody", "app.errorRetry", "common.loading"].map(
      (key) => [key, en[key]],
    ),
  );
  const bootstrapSource =
    "// Generated from the English catalog by gen-approval-reading.mjs.\nexport const bootstrapCopy = " +
    JSON.stringify(bootstrap, null, 2).replace(/\n}$/, ",\n}") +
    " as const;\n";
  const bootstrapPath = join(root, "src/i18n/bootstrapcopy_gen.ts");
  if (process.argv.includes("--check")) {
    if ((await readFile(bootstrapPath, "utf8")) !== bootstrapSource)
      throw new Error("Bootstrap copy is stale");
  } else await writeFile(bootstrapPath, bootstrapSource);
  const { previewFieldLabel } = await server.ssrLoadModule(
    "/src/lib/approvalpreviewfields.ts",
  );
  const { previewValue } = await server.ssrLoadModule(
    "/src/lib/approvalpreview.ts",
  );
  for (const id of server.environments.ssr.moduleGraph.idToModuleMap.keys()) {
    if (
      !id.startsWith(join(root, "src/")) ||
      id.includes("?") ||
      !/\.[cm]?[jt]sx?$/.test(id)
    )
      continue;
    const source = await readFile(id, "utf8");
    const name = id.slice(join(root, "src/").length);
    if (!name.startsWith("i18n/")) addCodes(source, name);
  }
  const { importPreviewFields } = await server.ssrLoadModule(
    "/src/lib/approvalpreviewimport.ts",
  );
  const importSource = await readFile(
    join(root, "src/lib/approvalpreviewimport.ts"),
    "utf8",
  );
  const outcomeCodes = [
    ...importSource
      .slice(
        importSource.indexOf("const OUTCOME_LABELS"),
        importSource.indexOf("export function"),
      )
      .matchAll(/^\s*([a-z_]+):/gm),
  ].map((match) => match[1]);
  if (outcomeCodes.length === 0)
    throw new Error("The import outcome catalog could not be read");
  const languages = {};
  for (const language of ["en", "de", "vi"]) {
    const module = await server.ssrLoadModule(`/src/i18n/${language}.ts`);
    const catalog = module[language];
    let translated = false;
    const t = (key, params = {}) => {
      translated = true;
      return (catalog[key] ?? key).replace(
        /\{(\w+)\}/g,
        (whole, key) => params[key] ?? whole,
      );
    };
    const labels = {};
    const coded = {};
    const defaults = {};
    for (const field of [...fields].sort(stable)) {
      labels[field] = previewFieldLabel(field, t);
      for (const kind of [
        "text",
        "lifecycle",
        "boolean",
        "empty",
        "execution",
      ]) {
        translated = false;
        const fallback = previewValue(
          { kind, text: "__unknown_preview_code__" },
          field,
          t,
          language,
        );
        if (translated && !fallback.includes("__unknown_preview_code__"))
          defaults[`${field}/${kind}`] = fallback;
        for (const code of [...values].sort(stable)) {
          translated = false;
          const text = previewValue({ kind, text: code }, field, t, language);
          if (
            translated &&
            text !== code &&
            text !== fallback &&
            !text.includes("{")
          )
            coded[`${field}/${kind}/${code}`] = text;
        }
      }
    }
    const outcomes = {};
    for (const outcome of outcomeCodes)
      outcomes[outcome] = importPreviewFields(
        [
          {
            line: 1,
            object: "company",
            source_key: "sample",
            outcome,
            changes: [],
            details: [],
            effects: [],
          },
        ],
        t,
        (value) => value.text,
        (effect) => effect,
      )[0].value;
    languages[language] = {
      labels,
      coded,
      defaults,
      outcomes,
      sections: {
        conflict: t("approval.preview.conflict"),
        file: t("import.step.files"),
        row: t("approval.import.row"),
      },
    };
  }
  // Each string value sits on its own line below its key. Git merges two edits
  // only when an unchanged line parts them, and the key parts neighbour values.
  const output = `${JSON.stringify({ languages }, null, 1).replace(
    /^( *)("(?:[^"\\]|\\.)*"): "/gm,
    '$1$2:\n$1 "',
  )}\n`;
  const path = join(
    root,
    "../backend/internal/modules/agents/previewcatalog_gen.json",
  );
  if (process.argv.includes("--check")) {
    if ((await readFile(path, "utf8")) !== output)
      throw new Error(
        "approval reading catalog is stale; run pnpm gen:approval-reading",
      );
  } else await writeFile(path, output);
} finally {
  await server.close();
}
