import { spawnSync } from "node:child_process";
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, symlinkSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { ensurePnpm } from "./ensure-pnpm.mjs";

const pin = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8")).packageManager;
const version = pin.slice("pnpm@".length).split("+")[0];
const placeholder = "# pnpm's native binary replaces this file during installation (see install.js)";

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "margince-pnpm-bootstrap-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const cache = join(root, "pnpm home");
  const tool = join(cache, ".tools", "pnpm", version);
  const directory = join(tool, "node_modules", "pnpm");
  mkdirSync(directory, { recursive: true });
  mkdirSync(join(tool, "bin"));
  mkdirSync(join(root, "bin"));
  writeFileSync(join(root, "package.json"), JSON.stringify({ packageManager: pin }));
  writeFileSync(join(directory, "package.json"), JSON.stringify({ name: "pnpm", version, type: "module" }));
  writeFileSync(join(directory, "pnpm"), placeholder);
  symlinkSync(join(directory, "pnpm"), join(tool, "bin", "pnpm"));
  writeFileSync(join(directory, "install.js"), `
    import { writeFileSync } from "node:fs";
    writeFileSync(new URL("pnpm", import.meta.url), "native executable");
    writeFileSync(new URL("ran", import.meta.url), process.env.npm_lifecycle_event);
  `);
  writeFileSync(join(root, "bin", "pnpm"), `#!${process.execPath}
    const fs = require("node:fs"), path = require("node:path");
    const pin = JSON.parse(fs.readFileSync("package.json", "utf8")).packageManager;
    const version = pin.slice(5).split("+")[0];
    const file = path.join(process.env.PNPM_HOME, ".tools", "pnpm", version, "node_modules", "pnpm", "pnpm");
    if (fs.readFileSync(file, "utf8") !== "native executable") {
      const diagnostic = process.env.TEST_BOOTSTRAP_ERROR || "Failed to switch pnpm: " + path.join(process.env.PNPM_HOME, ".tools", "pnpm", version, "bin", "pnpm") + " ENOEXEC";
      console[process.env.TEST_ERROR_STREAM === "stdout" ? "log" : "error"](diagnostic);
      process.exit(1);
    }
    console.log(process.env.TEST_PNPM_VERSION || version);
  `, { mode: 0o755 });
  const env = { ...process.env, PNPM_HOME: cache, PATH: `${join(root, "bin")}:${process.env.PATH}` };
  return { root, env, directory, tool };
}

const ran = (f) => existsSync(join(f.directory, "ran"));

test("a healthy pinned executable does not run the installer", (t) => {
  const f = fixture(t);
  writeFileSync(join(f.directory, "pnpm"), "native executable");
  ensurePnpm(f);
  assert.equal(ran(f), false);
});

test("a skipped native install runs the pinned installer and verifies its version", (t) => {
  const f = fixture(t);
  f.env.npm_lifecycle_event = "postinstall";
  ensurePnpm(f);
  assert.equal(readFileSync(join(f.directory, "ran"), "utf8"), "preinstall");
  assert.equal(readFileSync(join(f.directory, "pnpm"), "utf8"), "native executable");
  ensurePnpm(f);
});

test("the macOS default cache path works without PNPM_HOME", (t) => {
  const f = fixture(t);
  const home = join(f.root, "home");
  mkdirSync(join(home, "Library"), { recursive: true });
  symlinkSync(f.env.PNPM_HOME, join(home, "Library", "pnpm"));
  const configured = { ...f.env };
  delete configured.PNPM_HOME;
  const run = (command, args, options) => {
    return spawnSync(command, args, { ...options, env: { ...options.env, PNPM_HOME: join(home, "Library", "pnpm") } });
  };
  ensurePnpm({ root: f.root, env: configured, home, platform: "darwin", run });
  assert.equal(ran(f), true);
});

for (const field of ["name", "version"]) {
  test(`a different package ${field} cannot run an installer`, (t) => {
    const f = fixture(t);
    const manifest = { name: "pnpm", version, type: "module", [field]: "different" };
    writeFileSync(join(f.directory, "package.json"), JSON.stringify(manifest));
    assert.throws(() => ensurePnpm(f), /does not match/);
    assert.equal(ran(f), false);
  });
}

test("an unknown executable cannot run an installer", (t) => {
  const f = fixture(t);
  writeFileSync(join(f.directory, "pnpm"), "unrecognized executable");
  assert.throws(() => ensurePnpm(f), /not the known/);
  assert.equal(ran(f), false);
});

test("an installer symlink outside its package is refused", (t) => {
  const f = fixture(t);
  const installer = join(f.directory, "install.js");
  writeFileSync(join(f.root, "outside.js"), "process.exit(0)");
  rmSync(installer);
  symlinkSync(join(f.root, "outside.js"), installer);
  assert.throws(() => ensurePnpm(f), /does not match/);
  assert.equal(ran(f), false);
});

test("an installer failure stays a failure", (t) => {
  const f = fixture(t);
  writeFileSync(join(f.directory, "install.js"), "process.exit(7)");
  assert.throws(() => ensurePnpm(f), /installer failed/);
});

test("a repair that returns the wrong version stays a failure", (t) => {
  const f = fixture(t);
  f.env.TEST_PNPM_VERSION = "different";
  assert.throws(() => ensurePnpm(f), /still does not run at the pinned version/);
  assert.equal(ran(f), true);
});

test("a missing pnpm command never runs a cached installer", (t) => {
  const f = fixture(t);
  const error = Object.assign(new Error("pnpm not found"), { code: "ENOENT" });
  assert.throws(() => ensurePnpm({ ...f, run: () => ({ error }) }), /pnpm not found/);
  assert.equal(ran(f), false);
});

test("an executable at a different version is refused without a repair", (t) => {
  const f = fixture(t);
  writeFileSync(join(f.directory, "pnpm"), "native executable");
  f.env.TEST_PNPM_VERSION = "different";
  assert.throws(() => ensurePnpm(f), /does not match/);
  assert.equal(ran(f), false);
});


test("an unrelated startup failure never runs an installer", (t) => {
  const f = fixture(t);
  f.env.TEST_BOOTSTRAP_ERROR = "network configuration failed";
  assert.throws(() => ensurePnpm(f), /does not name this cached placeholder/);
  assert.equal(ran(f), false);
});

test("a placeholder failure reported on stdout can be repaired", (t) => {
  const f = fixture(t);
  f.env.TEST_ERROR_STREAM = "stdout";
  ensurePnpm(f);
  assert.equal(ran(f), true);
});


test("a version directory outside the tool cache is refused", (t) => {
  const f = fixture(t);
  const outside = join(f.root, "outside-tool");
  renameSync(f.tool, outside);
  symlinkSync(outside, f.tool);
  assert.throws(() => ensurePnpm(f), /does not match/);
  assert.equal(ran(f), false);
});

test("a package directory outside the pinned tool is refused", (t) => {
  const f = fixture(t);
  const outside = join(f.root, "outside-package");
  renameSync(f.directory, outside);
  symlinkSync(outside, f.directory);
  assert.throws(() => ensurePnpm(f), /does not match/);
  assert.equal(ran(f), false);
});

test("a bin link to another executable is refused", (t) => {
  const f = fixture(t);
  const other = join(f.directory, "other-pnpm");
  writeFileSync(other, placeholder);
  rmSync(join(f.tool, "bin", "pnpm"));
  symlinkSync(other, join(f.tool, "bin", "pnpm"));
  assert.throws(() => ensurePnpm(f), /does not match/);
  assert.equal(ran(f), false);
});

test("a large executable is never loaded as a placeholder", (t) => {
  const f = fixture(t);
  writeFileSync(join(f.directory, "pnpm"), placeholder + "x".repeat(4096));
  assert.throws(() => ensurePnpm(f), /not the known/);
  assert.equal(ran(f), false);
});
