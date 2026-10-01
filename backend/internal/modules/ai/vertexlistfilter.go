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

// uncallable names catalog entries no binding here can call: speech, live
// audio and image generation share the gemini- prefix with the text models.
var uncallable = []string{"-tts", "native-audio", "live-", "-image"}

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
// since an outage must not empty the picker.
func (s *RoutingStore) servedOnly(ctx context.Context, client *geminiClient, location string, models []model.Info) []model.Info {
	callable := make([]model.Info, 0, len(models))
	for _, m := range models {
		if !isUncallable(m.ID) {
			callable = append(callable, m)
		}
	}
	served := s.served.lookup(location, s.clock())
	if served == nil {
		served = probeLocation(ctx, client.relocated(location), location, callable)
		s.served.remember(location, servedAnswer{at: s.clock(), served: served})
	}
	kept := make([]model.Info, 0, len(callable))
	for _, m := range callable {
		if keep, asked := served[m.ID]; !asked || keep {
			kept = append(kept, m)
		}
	}
	return kept
}

func isUncallable(id string) bool {
	for _, marker := range uncallable {
		if strings.Contains(id, marker) {
			return true
		}
	}
	return false
}

// probeLocation asks location about each model, probeFanout at a time, and
// answers which it serves: false only where Google said it does not.
func probeLocation(ctx context.Context, client *geminiClient, location string, models []model.Info) map[string]bool {
	served := make(map[string]bool, len(models))
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, probeFanout)
	for _, m := range models {
		wg.Add(1)
		slots <- struct{}{}
		go func(m model.Info) {
			defer func() { <-slots; wg.Done() }()
			err := probeOnce(ctx, client, vertexProbe{location: location, model: m.ID, lane: m.Lane})
			mu.Lock()
			served[m.ID] = !errors.Is(err, errModelNotFound)
			mu.Unlock()
		}(m)
	}
	wg.Wait()
	return served
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
