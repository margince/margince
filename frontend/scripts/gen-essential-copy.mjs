import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const server = await createServer({
  root,
  configFile: false,
  optimizeDeps: { noDiscovery: true },
  server: { middlewareMode: true, hmr: false, ws: false },
  appType: "custom",
});
try {
  const { essentialKeys } = await server.ssrLoadModule(
    "/scripts/lib/essential-copy.ts",
  );
  const { LOCALES } = await server.ssrLoadModule("/src/i18n/locale.ts");
  const catalogs = {};
  for (const locale of LOCALES) {
    const module = await server.ssrLoadModule(`/src/i18n/${locale}.ts`);
    catalogs[locale] = module[locale];
  }
  const keys = essentialKeys(root);
  const outputs = new Map([
    ...LOCALES.map((locale) => [
      join(root, "src/i18n/essential", `${locale}_gen.json`),
      Object.fromEntries(
        Object.entries(catalogs[locale]).filter(([key]) => keys.has(key)),
      ),
    ]),
  ]);
  for (const [file, value] of outputs) {
    const source = `${JSON.stringify(value, null, 2)}\n`;
    if (process.argv.includes("--check")) {
      if (!existsSync(file) || readFileSync(file, "utf8") !== source)
        throw new Error(
          "Initial catalog copy is stale; run pnpm gen:essential-copy",
        );
    } else {
      mkdirSync(dirname(file), { recursive: true });
      writeFileSync(file, source);
    }
  }
} finally {
  await server.close();
}
