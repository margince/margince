// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/jackc/pgx/v5/pgxpool"

// WithCardPhotoReader binds the lane that reads a photo of business cards.
//
// Its absence is a 501, not a template. With no model nothing can read a
// photograph, and a guess at the print would be worse than saying so.
func WithCardPhotoReader(lane documentCompleter) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		if lane == nil {
			return
		}
		s.contactsHandlers = s.contactsHandlers.WithCardPhotoReader(cardPhotoReader(lane))
	}
}
