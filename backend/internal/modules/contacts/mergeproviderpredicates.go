// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

const mergePaidProviderStates = `('submitting','in_progress','submission_unknown')`

const mergePaidProviderTwin = `EXISTS (
 SELECT 1 FROM provider_run o WHERE o.contact_id = @other AND o.provider = s.provider
 AND o.input_fingerprint = s.input_fingerprint AND o.state IN ` + mergePaidProviderStates + `)`

const mergeProviderCategoryTwin = `EXISTS (
 SELECT 1 FROM provider_run_category mine JOIN provider_run_category theirs
 ON theirs.provider = mine.provider AND theirs.category = mine.category
 JOIN provider_run origin ON origin.id = theirs.run_id
 WHERE mine.run_id = q.id AND theirs.contact_id = @source
 AND origin.state IN ` + mergePaidProviderStates + `)`
