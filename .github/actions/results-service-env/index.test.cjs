// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
'use strict'
// Runs index.cjs as the runner runs it: a node process with the job's env.
const { test } = require('node:test')
const assert = require('node:assert/strict')
const { spawnSync } = require('node:child_process')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')

function run(env) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'results-env-'))
  const githubEnv = path.join(dir, 'env')
  fs.writeFileSync(githubEnv, 'EARLIER=kept\n')
  const result = spawnSync(process.execPath, [path.join(__dirname, 'index.cjs')], {
    env: { PATH: process.env.PATH, GITHUB_ENV: githubEnv, ...env },
    encoding: 'utf8',
  })
  return { ...result, written: fs.readFileSync(githubEnv, 'utf8') }
}

test('hands the token and the address to later steps, and masks the token', () => {
  const r = run({ ACTIONS_RUNTIME_TOKEN: 'tok', ACTIONS_RESULTS_URL: 'https://results.example/' })
  assert.equal(r.status, 0)
  assert.equal(r.written, 'EARLIER=kept\nACTIONS_RUNTIME_TOKEN=tok\nACTIONS_RESULTS_URL=https://results.example/\n')
  assert.match(r.stdout, /^::add-mask::tok$/m)
})

test('fails, and writes nothing, when the runner gave no results service', () => {
  for (const env of [{ ACTIONS_RESULTS_URL: 'https://results.example/' }, { ACTIONS_RUNTIME_TOKEN: 'tok' }]) {
    const r = run(env)
    assert.equal(r.status, 1)
    assert.equal(r.written, 'EARLIER=kept\n')
    assert.match(r.stdout, /^::error::/m)
  }
})
