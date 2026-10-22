// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
)

const (
	companyContextRolloutOff        = "off"
	companyContextRolloutRead       = "read"
	companyContextRolloutTasks      = "tasks"
	companyContextRolloutOnboarding = "onboarding"
)

// CompanyContextRollout stages company knowledge: each stage includes the one
// before it, and moving back disables surfaces without deleting confirmed data.
var CompanyContextRollout = config.Define(config.Spec[string]{
	Key: "company_context.rollout", Env: "MARGINCE_COMPANY_CONTEXT_ROLLOUT", Roles: []config.Role{config.RoleAPI},
	Type:    config.Enum(companyContextRolloutOff, companyContextRolloutRead, companyContextRolloutTasks, companyContextRolloutOnboarding),
	Default: config.Some(companyContextRolloutOnboarding),
	Doc:     "Company-context stage: off, read (reads and settings), tasks (AI tasks get it) or onboarding (the first-run flow too).",
})

// CompanyContextTasks reports whether declared AI tasks may receive company
// data under the loaded rollout.
func CompanyContextTasks(c *config.Config) bool {
	return companyContextTasksEnabled(config.Val(c, CompanyContextRollout))
}

// WithCompanyContextRollout gives the HTTP surfaces the already-validated
// operator capability. The composition root is the only config reader.
func WithCompanyContextRollout(rollout string) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.rollout = rollout
		s.companyContextRollout = rollout
		if s.proposal != nil {
			s.proposal.rollout = rollout
		}
	}
}

// publishCompanyContextAvailability tells /me whether the company-context
// rollout has typed reads active, from the SAME rollout the endpoints gate on.
//
// Called after every option has run rather than inside the option above, and
// that ordering is the whole point. The endpoints do not depend on the option
// running — an unset rollout means every stage is on, which
// companyContextReadEnabled says and GetCompanyContextCapabilities re-derives —
// while the injected boolean would be the zero value, false. Set inside the
// option, the two would agree only for a server that has the option; one
// without it would report the surface absent while serving it.
//
// Reading s.companyContextRollout here instead means both sides resolve the same
// field through the same predicate, whatever ran. The agreement is then a
// property of the code rather than of an operator remembering a second switch.
func (s *Server) publishCompanyContextAvailability() {
	s.authHandlers = s.WithCompanyContextAvailable(companyContextReadEnabled(s.companyContextRollout))
}

func companyContextReadEnabled(rollout string) bool {
	return rollout == "" || rollout == companyContextRolloutRead || rollout == companyContextRolloutTasks || rollout == companyContextRolloutOnboarding
}

func companyContextTasksEnabled(rollout string) bool {
	return rollout == "" || rollout == companyContextRolloutTasks || rollout == companyContextRolloutOnboarding
}

func companyContextOnboardingEnabled(rollout string) bool {
	return rollout == "" || rollout == companyContextRolloutOnboarding
}
