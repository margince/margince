// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
'use strict'
const fs = require('node:fs')

const token = process.env.ACTIONS_RUNTIME_TOKEN
const url = process.env.ACTIONS_RESULTS_URL
if (!token || !url) {
  console.log('::error::ACTIONS_RUNTIME_TOKEN or ACTIONS_RESULTS_URL is not set; the runner gave this action no results service')
  process.exit(1)
}
console.log(`::add-mask::${token}`)
fs.appendFileSync(process.env.GITHUB_ENV, `ACTIONS_RUNTIME_TOKEN=${token}\nACTIONS_RESULTS_URL=${url}\n`)
