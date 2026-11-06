// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// voiceInferenceReply is what the voice builder's model answers: a voice that
// cites no sample, which the builder's checks accept.
const voiceInferenceReply = `{"identity_summary":"Plain and short.","thinking_pattern":"Point first.",` +
	`"observed_obsessions":[],"directness":"Direct.","structure":"Short paragraphs.","openings":[],"closings":[],` +
	`"vocabulary":[],"avoid":[],"signature_moves":[],"register_notes":[],"evidence":[]}`

// voiceFixtures fill the calls on a profile's built versions.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var voiceFixtures = map[string]func(p *recordPool) map[string]any{
	"apply_voice_version":  onVoiceVersion("voice_candidate", "candidate"),
	"reject_voice_version": onVoiceVersion("voice_candidate", "candidate"),
	"rollback_voice_version": func(p *recordPool) map[string]any {
		profile := p.take("voice_rolled_forward")
		return map[string]any{"id": profile, "profileVersion": p.fact(profile, "previous")}
	},
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func onVoiceVersion(kind, fact string) func(p *recordPool) map[string]any {
	return func(p *recordPool) map[string]any {
		profile := p.take(kind)
		return map[string]any{"id": profile, "profileVersion": p.fact(profile, fact), "if_version": versionOf(p, profile)}
	}
}

// voiceProfileRow is the part of a voice profile the version seeds read.
type voiceProfileRow struct {
	ProfileVersion   int  `json:"profile_version"`
	CandidateVersion *int `json:"candidate_version"`
}

// voiceSeeders make a profile with a built version and a candidate after it.
// The user's live profile is archived, and a new one is built twice from one
// document. The first build is the voice; a rebuild with no sample held out
// waits for review.
func voiceSeeders(s sweepSeed) map[string]func() string {
	read := func(profile string) voiceProfileRow {
		var row voiceProfileRow
		s.call("GET", "/v1/voice-profiles/"+profile, nil, nil, &row)
		return row
	}
	build := func(profile string) {
		var queued struct {
			ID string `json:"id"`
		}
		s.call("POST", "/v1/voice-profiles/"+profile+"/builds", integration.AnyMap{"reason": "manual"}, nil, &queued)
		worker, err := compose.VoiceBuildWorkerForTest(s.e.Pool, sweepVoiceModel)
		if err != nil {
			s.t.Fatalf("building the voice build worker: %v", err)
		}
		for _, job := range importjobtest.Queued(s.t, s.e, compose.VoiceBuildArgs{}.Kind()) {
			var args compose.VoiceBuildArgs
			if json.Unmarshal(job.EncodedArgs, &args) == nil && args.BuildID == queued.ID {
				importjobtest.WorkJob(s.t, s.e, worker, job, 0)
			}
		}
	}
	candidate := func() string {
		s.call("DELETE", "/v1/voice-profiles/"+s.create["voice_profile"](), nil, nil, nil)
		profile := s.post("/v1/voice-profiles", integration.AnyMap{})
		s.call("POST", "/v1/voice-profiles/"+profile+"/sources", integration.AnyMap{
			"kind": "document", "register": "long_form", "format": "text", "source_label": "Role sweep",
			"source_ref": "sweep-" + ids.NewV7().String(), "content": strings.Repeat("word ", 800),
		}, nil, nil)
		build(profile)
		build(profile)
		row := read(profile)
		if row.CandidateVersion == nil {
			s.t.Fatalf("voice profile %s has no candidate after a rebuild: %+v", profile, row)
		}
		// A decision on a version is pinned to the version in force.
		s.pool.note(profile, "version", strconv.Itoa(row.ProfileVersion))
		s.pool.note(profile, "candidate", strconv.Itoa(*row.CandidateVersion))
		s.pool.note(profile, "previous", strconv.Itoa(row.ProfileVersion))
		return profile
	}
	return map[string]func() string{
		"voice_candidate": candidate,
		// The candidate is applied, so the version before it is one to roll back to.
		"voice_rolled_forward": func() string {
			profile := candidate()
			version, _ := strconv.Atoi(s.pool.fact(profile, "version"))
			s.call("POST", "/v1/voice-profiles/"+profile+"/versions/"+s.pool.fact(profile, "candidate")+"/apply", nil, pinned(version), nil)
			return profile
		},
	}
}
