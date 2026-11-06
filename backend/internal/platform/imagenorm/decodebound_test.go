// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"runtime"
	"testing"
	"testing/synctest"
	"time"
)

// progressiveJPEGHeader is a valid one-component progressive JPEG up to its
// first scan. Its one AC code ends the next 2^14 blocks, so a scan covers the
// canvas from a few zero bytes.
func progressiveJPEGHeader(edge int) []byte {
	side := []byte{byte(edge >> 8), byte(edge)}
	out := []byte{0xff, jpegSOI}
	out = append(out, 0xff, 0xdb, 0x00, 0x43, 0x00)
	out = append(out, bytes.Repeat([]byte{0x01}, 64)...)
	out = append(out, 0xff, 0xc2, 0x00, 0x0b, 0x08, side[0], side[1], side[0], side[1], 0x01, 0x01, 0x11, 0x00)
	for _, table := range [][2]byte{{0x00, 0x00}, {0x10, 0xe0}} {
		out = append(out, 0xff, 0xc4, 0x00, 0x14, table[0], 0x01)
		out = append(out, make([]byte, 15)...)
		out = append(out, table[1])
	}
	return out
}

// progressiveJPEG adds scans AC scans to progressiveJPEGHeader, each marker
// preceded by markerPrefix, and closes the file.
func progressiveJPEG(edge, scans int, markerPrefix []byte) []byte {
	blocks := (edge / 8) * (edge / 8)
	eobCodes := (blocks + 1<<14 - 1) >> 14
	scanData := make([]byte, (eobCodes*15+7)/8+2)
	out := progressiveJPEGHeader(edge)
	for range scans {
		out = append(out, markerPrefix...)
		out = append(out, 0xff, jpegSOS, 0x00, 0x08, 0x01, 0x01, 0x00, 0x01, 0x3f, 0x00)
		out = append(out, scanData...)
	}
	return append(out, 0xff, jpegEOI)
}

func baselineJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 5), B: 90, A: 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, nil); err != nil {
		t.Fatalf("encoding the baseline JPEG: %v", err)
	}
	return out.Bytes()
}

func TestDecodeReadsJPEGsWithTheScanCountsEncodersWrite(t *testing.T) {
	cases := map[string][]byte{
		"baseline":                             baselineJPEG(t),
		"progressive with ten scans":           progressiveJPEG(256, 10, nil),
		"a small picture with a hundred scans": progressiveJPEG(256, 100, nil),
		"the largest canvas at its scan cap":   progressiveJPEG(2896, jpegScanCap(2896, 2896), nil),
		"scan markers after fill bytes":        progressiveJPEG(256, 10, []byte{0xff, 0xff}),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(src); err != nil {
				t.Fatalf("a JPEG within the scan cap must decode: %v", err)
			}
		})
	}
}

func TestDecodeRefusesAJPEGWithMoreScansThanAnyEncoderWrites(t *testing.T) {
	cases := map[string][]byte{
		"one scan over a small canvas's cap":     progressiveJPEG(256, jpegScanCap(256, 256)+1, nil),
		"one scan over the largest canvas's cap": progressiveJPEG(2896, jpegScanCap(2896, 2896)+1, nil),
		"two thousand scans of a large canvas":   progressiveJPEG(2048, 2000, nil),
		"scans hidden behind fill bytes":         progressiveJPEG(2048, 2000, []byte{0xff, 0xff}),
		"scans after stray bytes and FF00":       progressiveJPEG(2048, 2000, []byte{0x5a, 0xff, 0x00, 0x5a}),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			_, err := Decode(src)
			elapsed := time.Since(start)
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("a JPEG over the scan cap must be unsupported, got %v", err)
			}
			if elapsed > time.Second {
				t.Fatalf("refusing %d bytes took %v; the refusal must come before any scan is decoded", len(src), elapsed)
			}
		})
	}
}

func TestDecodeRefusesEveryTruncationOfAProgressiveJPEG(t *testing.T) {
	full := progressiveJPEG(256, 10, nil)
	for cut := len(progressiveJPEGHeader(256)); cut < len(full); cut++ {
		if _, err := Decode(full[:cut]); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("the first %d of %d bytes must be unsupported, got %v", cut, len(full), err)
		}
	}
}

func TestDecodeRefusesAGarbledJPEGMarkerStream(t *testing.T) {
	header := progressiveJPEGHeader(256)
	firstScan := len(header)
	withScanLength := func(high, low byte) []byte {
		src := progressiveJPEG(256, 3, nil)
		src[firstScan+2], src[firstScan+3] = high, low
		return src
	}
	cases := map[string][]byte{
		"a segment length past the end": withScanLength(0xff, 0xf0),
		"a segment length under two":    withScanLength(0x00, 0x01),
		"stuffed zeros with no marker":  append(header, bytes.Repeat([]byte{0xff, 0x00}, 4096)...),
		"fill bytes with no marker":     append(header, bytes.Repeat([]byte{0xff}, 4096)...),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(src); !errors.Is(err, ErrUnsupported) {
				t.Fatalf("a garbled marker stream must be unsupported, got %v", err)
			}
		})
	}
}

func TestDecodeWaitsForASlotWhileEverySlotIsHeld(t *testing.T) {
	if procs := runtime.GOMAXPROCS(0); cap(decodeSlots) < 1 || (procs > 1 && cap(decodeSlots) >= procs) {
		t.Fatalf("%d decode slots on %d cores must leave a core free and keep at least one slot", cap(decodeSlots), procs)
	}
	src := baselineJPEG(t)
	synctest.Test(t, func(t *testing.T) {
		holdEverySlot(t, 2, 3, 0)

		const callers = 3
		results := make(chan error, callers)
		for range callers {
			go func() {
				_, err := Decode(src)
				results <- err
			}()
		}
		synctest.Wait()
		if finished := len(results); finished != 0 {
			t.Fatalf("%d decodes ran while every slot was held", finished)
		}

		<-decodeSlots
		synctest.Wait()
		for range callers {
			if err := <-results; err != nil {
				t.Fatalf("a decode that waited for its slot failed: %v", err)
			}
		}
		if held := len(decodeSlots); held != cap(decodeSlots)-1 {
			t.Fatalf("%d slots held after the decodes finished, want only the test's %d", held, cap(decodeSlots)-1)
		}
	})
}

// holdEverySlot swaps in a small pool of held slots and a queue with queued places taken.
func holdEverySlot(t *testing.T, slots, queue, queued int) {
	t.Helper()
	processSlots, processQueue := decodeSlots, decodeQueue
	decodeSlots, decodeQueue = make(chan struct{}, slots), make(chan struct{}, queue)
	t.Cleanup(func() { decodeSlots, decodeQueue = processSlots, processQueue })
	for range slots {
		decodeSlots <- struct{}{}
	}
	for range queued {
		decodeQueue <- struct{}{}
	}
}

func TestADecodeThatWaitsPastItsBoundIsRefusedBusy(t *testing.T) {
	src := baselineJPEG(t)
	synctest.Test(t, func(t *testing.T) {
		holdEverySlot(t, 1, 1, 0)
		start := time.Now()
		if _, err := Decode(src); !errors.Is(err, ErrDecodeBusy) {
			t.Fatalf("a decode that never got a slot = %v, want ErrDecodeBusy", err)
		}
		if waited := time.Since(start); waited != decodeSlotWait {
			t.Fatalf("the decode waited %v, want exactly %v", waited, decodeSlotWait)
		}
		if queued := len(decodeQueue); queued != 0 {
			t.Fatalf("a refused decode left %d places taken in the queue", queued)
		}
	})
}

func TestADecodeFindingTheQueueFullIsRefusedAtOnce(t *testing.T) {
	src := baselineJPEG(t)
	synctest.Test(t, func(t *testing.T) {
		holdEverySlot(t, 1, 2, 2)
		start := time.Now()
		if _, err := Decode(src); !errors.Is(err, ErrDecodeBusy) || errors.Is(err, ErrUnsupported) {
			t.Fatalf("a decode behind a full queue = %v, want ErrDecodeBusy and not a refusal of the bytes", err)
		}
		if waited := time.Since(start); waited != 0 {
			t.Fatalf("a decode behind a full queue waited %v, want no wait", waited)
		}
	})
}
