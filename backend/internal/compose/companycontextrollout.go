// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/jackc/pgx/v5/pgxpool"

const (
	companyContextRolloutOff        = "off"
	companyContextRolloutRead       = "read"
	companyContextRolloutTasks      = "tasks"
	companyContextRolloutOnboarding = "onboarding"
)

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

// publishCompanyContextAvailability tells /me whether the Company settings page
// exists here, from the SAME rollout the endpoints gate on.
//
// Called after every option has run rather than inside the option above, and
// that ordering is the whole point. The endpoints do not depend on the option
// running — an unset rollout means every stage is on, which
// companyContextReadEnabled says and GetCompanyContextCapabilities re-derives —
// while the injected boolean would be the zero value, false. So setting it
// inside the option made the two agree only for a server that HAD the option,
// and a server without one advertised the page as absent while serving it.
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
