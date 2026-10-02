// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Google's publisher catalog is one global list, and a location serves only
// part of it; nothing lists what a location serves, so the picker asks the
// location about each model, the way a save does, and offers only those.

// servedFor is how long what a location was found to serve is reused: the
// picker opens often, and a location's lineup changes on Google's release
// days, not between two opens.
const servedFor = time.Hour

// probeFanout bounds how many models are asked about at once.
const probeFanout = 8

// probeBudget bounds the whole round, so one slow location cannot hold a
// picker open until the server's write timeout cancels it.
const probeBudget = 20 * time.Second

// uncallable names catalog entries no binding here can call: speech, live
// audio and image generation share the gemini- prefix with the text models,
// and the multimodal embedder takes no plain text instance.
var uncallable = []string{"-tts", "native-audio", "live-", "-image", "multimodalembedding"}

type servedAtLocation struct {
	mu    sync.Mutex
	known map[string]servedAnswer
}

type servedAnswer struct {
	at     time.Time
	served map[string]bool
}

func (s *RoutingStore) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// servedOnly keeps the listed models location serves. Only a definite "not
// served" drops one: a probe Google did not answer keeps the model on offer,
// since an outage must not empty the picker. A round is reused only when every
// probe was answered; an embedder's probe is one billed embed call. It reports
// whether the list is exact: every model's answer known, fresh or reused.
func (s *RoutingStore) servedOnly(ctx context.Context, client *geminiClient, location string, models []model.Info) ([]model.Info, bool) {
	callable := make([]model.Info, 0, len(models))
	for _, m := range models {
		if !isUncallable(m.ID) {
			callable = append(callable, m)
		}
	}
	key := servedKey(client, location)
	served := s.served.lookup(key, s.clock())
	answered := served != nil
	if served == nil {
		served, answered = probeLocation(ctx, client.relocated(location), location, callable)
		if answered {
			s.served.remember(key, servedAnswer{at: s.clock(), served: served})
		}
	}
	kept := make([]model.Info, 0, len(callable))
	exact := answered
	for _, m := range callable {
		keep, asked := served[m.ID]
		exact = exact && asked
		if !asked || keep {
			kept = append(kept, m)
		}
	}
	return kept, exact
}

func isUncallable(id string) bool {
	for _, marker := range uncallable {
		if strings.Contains(id, marker) {
			return true
		}
	}
	return false
}

// servedKey names whose answer it is: what a location serves differs by
// Google project, so a key from another project is asked afresh.
func servedKey(client *geminiClient, location string) string {
	vertex, _ := client.transport.(vertexTransport)
	return vertex.projectID + "/" + location
}

// probeLocation asks location about each model, probeFanout at a time, and
// answers which it serves — false only where Google said it does not — and
// whether Google answered every probe.
func probeLocation(ctx context.Context, client *geminiClient, location string, models []model.Info) (map[string]bool, bool) {
	ctx, cancel := context.WithTimeout(ctx, probeBudget)
	defer cancel()
	served := make(map[string]bool, len(models))
	answered := true
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, probeFanout)
	for _, m := range models {
		if ctx.Err() != nil {
			mu.Lock()
			answered = false
			mu.Unlock()
			break
		}
		wg.Add(1)
		slots <- struct{}{}
		go func(m model.Info) {
			defer func() { <-slots; wg.Done() }()
			err := probeOnce(ctx, client, vertexProbe{location: location, model: m.ID, lane: m.Lane})
			mu.Lock()
			defer mu.Unlock()
			served[m.ID] = !errors.Is(err, errModelNotFound)
			if err != nil && !errors.Is(err, errModelNotFound) {
				answered = false
			}
		}(m)
	}
	wg.Wait()
	return served, answered && ctx.Err() == nil
}

func (c *servedAtLocation) lookup(location string, now time.Time) map[string]bool {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	answer, ok := c.known[location]
	if !ok || now.Sub(answer.at) >= servedFor {
		return nil
	}
	return answer.served
}

func (c *servedAtLocation) remember(location string, answer servedAnswer) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.known == nil {
		c.known = map[string]servedAnswer{}
	}
	c.known[location] = answer
}
