// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The public origin a send builds its links on, carried to the roles that fire
// one.
//
// SendPath already holds this pair for the api's immediate send. The scheduled
// worker is composed from JobsConfig rather than from a SendPath, so the pair
// travels on its own rather than by handing the whole send path to a role that
// needs two fields of it — and it stays a PAIR, because a base URL judged under
// the wrong posture is the failure netguard exists to catch: a loopback origin
// is a working dev stack or a link the recipient cannot open, and only the
// environment says which.

import "github.com/margince/margince/backend/internal/shared/runtimeenv"

// SendOrigin is the installation's public base URL and the posture it is judged
// under. The zero value is an unconfigured origin in production posture, which
// is the direction that fails safe.
type SendOrigin struct {
	PublicBaseURL string
	Environment   runtimeenv.Environment
}

// sendPath renders the origin as the send path a store is built from. Spelled
// here so the two fields cannot be copied across one at a time.
func (o SendOrigin) sendPath() SendPath {
	return SendPath{PublicBaseURL: o.PublicBaseURL, Environment: o.Environment}
}
