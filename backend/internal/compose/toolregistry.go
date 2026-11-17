// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The agent tool surface, built from the server's current state.
//
// It lives beside server.go rather than in it because server.go is the
// INVENTORY — what this process serves — and this is one assembly step over
// that inventory. Splitting it also gives the inventory room to grow: the file
// had reached the length where a reader stops holding it at once, and every new
// handler set is one more line in it.

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// buildToolRegistry builds the agent tool surface from the server's current
// state. Options only record onto the Server, and New calls this after the
// last one, so the surface holds the wiring of every option.
func (s *Server) buildToolRegistry(pool *pgxpool.Pool) {
	// The gate and the registry take the SAME meter pointer: one refuses on the
	// bound, the other pays into it, and a surface where those were two
	// counters would step an agent up against a number nothing was charging.
	opts := []agents.RegistryOption{agents.WithVolumeCharger(s.volumeMeter), agents.WithCostShare(s.volumeMeter)}
	opts = append(opts, s.registryForTest...)
	if s.trial != nil {
		opts = append(opts, agents.WithInstallationLock(s.trial.Lock))
	}
	s.toolRegistry = registryWithGate(InstallationDB(pool),
		auth.NewGate(identity.NewService(pool), auth.WithVolumeMeter(s.volumeMeter)),
		s.replyDrafter, s.send, companyEnricher{srv: s},
		s.retrievalEmbedder, s.transcriptOnLanding, importsFor(s),
		// The SERVER's brief service, not a second one built from the pool: the
		// model lane is bound to that instance, so a fresh service here would
		// serve agents the deterministic floor while the contact page got prose.
		meetingBriefReader(s.meetingBriefSvc), s.log, registryFeatures{coreFeatures: s.coreFeatures, lists: s.listsEnabled, firstDrafts: &s.firstDrafts, served: s, extensions: s.extensions},
		opts...)
}
