// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import "io"

// Cap capacity as well as length; ReadAll may grow beyond a large response limit.
func readResponseBody(reader io.Reader, limit int64, knownLength bool) ([]byte, error) {
	ceiling := int(limit + 1)
	capacity := min(ceiling, 32<<10)
	if knownLength {
		capacity = ceiling
	}
	body := make([]byte, 0, capacity)
	emptyReads := 0
	for {
		if len(body) == cap(body) {
			if len(body) == ceiling {
				return body, nil
			}
			next := make([]byte, len(body), min(ceiling, 2*cap(body)))
			copy(next, body)
			body = next
		}
		read, err := reader.Read(body[len(body):cap(body)])
		body = body[:len(body)+read]
		if read == 0 && err == nil {
			emptyReads++
			if emptyReads >= 100 {
				return nil, io.ErrNoProgress
			}
		} else {
			emptyReads = 0
		}
		if err == io.EOF {
			return body, nil
		}
		if err != nil {
			return nil, err
		}
	}
}
