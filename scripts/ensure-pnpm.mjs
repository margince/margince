import { spawnSync } from "node:child_process";
import { readFileSync, realpathSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const repository = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const placeholder = "# pnpm's native binary replaces this file during installation";

function inside(parent, child) {
  const path = relative(parent, child);
  return path !== ".." && !path.startsWith(`..${sep}`) && !isAbsolute(path);
}

function installerFor(home, version) {
  const tools = realpathSync(join(home, ".tools", "pnpm"));
  const tool = realpathSync(join(tools, version));
  const directory = realpathSync(join(tool, "node_modules", "pnpm"));
  const installer = realpathSync(join(directory, "install.js"));
  const binary = realpathSync(join(tool, "bin", "pnpm"));
  const manifest = JSON.parse(readFileSync(join(directory, "package.json"), "utf8"));
  if (!inside(tools, tool) || !inside(tool, directory) || !inside(directory, installer) ||
      binary !== join(directory, "pnpm") || manifest.name !== "pnpm" || manifest.version !== version) {
    throw new Error("cached package does not match the pinned pnpm package and path");
  }
  // Bound diagnosis reads so an installed native executable is never loaded.
  if (statSync(binary).size > 4096 || !readFileSync(binary, "utf8").startsWith(placeholder)) {
    throw new Error("cached executable is not the known pnpm installer placeholder");
  }
  return { directory, installer, binDirectory: realpathSync(join(tool, "bin")) };
}

export function ensurePnpm({ root = repository, env = process.env, home = homedir(),
  platform = process.platform, run = spawnSync } = {}) {
  const pin = JSON.parse(readFileSync(join(root, "package.json"), "utf8")).packageManager;
  const match = /^pnpm@(\d+\.\d+\.\d+)(?:\+[a-z0-9]+\.[0-9a-f]+)?$/.exec(pin);
  if (!match) throw new Error("package.json must pin an exact pnpm version");
  const version = match[1];
  const options = { cwd: root, env, encoding: "utf8" };
  const before = run("pnpm", ["--version"], options);
  if (before.error) throw before.error;
  if (before.status === 0) {
    if (before.stdout.trim() !== version) throw new Error("pnpm does not match package.json's pin");
    return;
  }
  const cache = env.PNPM_HOME || (platform === "darwin" ? join(home, "Library", "pnpm") : undefined);
  try {
    if (!cache) throw new Error("no supported pnpm cache path is configured");
    const { directory, installer, binDirectory } = installerFor(cache, version);
    const diagnostic = `${before.stdout}\n${before.stderr}`;
    const expected = join(cache, ".tools", "pnpm", version, "bin");
    if (!diagnostic.includes("ENOEXEC") ||
        ![expected, binDirectory].some((path) => diagnostic.includes(path))) {
      throw new Error("the startup failure does not name this cached placeholder");
    }
    console.error("pnpm bootstrap: completing the pinned package's native binary installation");
    // pnpm postinstall only relinks Windows shims; native setup needs preinstall.
    const repaired = run(process.execPath, [installer], {
      cwd: directory, env: { ...env, npm_lifecycle_event: "preinstall" }, stdio: "inherit", timeout: 120000,
    });
    if (repaired.error) throw repaired.error;
    if (repaired.status !== 0) throw new Error("the pinned pnpm installer failed");
    const after = run("pnpm", ["--version"], options);
    if (after.error) throw after.error;
    if (after.status !== 0 || after.stdout.trim() !== version) {
      throw new Error("pnpm still does not run at the pinned version");
    }
  } catch (error) {
    throw new Error(`Pinned pnpm could not start. Reinstall the pinned package and run make install again. Automatic repair only handles pnpm self-management caches.\nCache checked: ${cache || "none; set PNPM_HOME for that cache"}\n${before.stdout.trim()}\n${before.stderr.trim()}\nRepair stopped: ${error.message}`, { cause: error });
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    ensurePnpm();
  } catch (error) {
    console.error(`pnpm bootstrap: ${error.message}`);
    process.exitCode = 1;
  }
}
