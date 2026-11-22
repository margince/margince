// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// How accurate the forecast has been. Its frozen chances are set against the
// deals that have since closed. Its frozen landings are set against the
// quarters that have since ended.

import (
	"math"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// CalibrationLeadDays is how long before a deal closed its chance is read.
const CalibrationLeadDays = 14

// calibrationBandWidth is the width of one band, in percent.
const calibrationBandWidth = 10

// SettledChance is one closed deal: the chance it was weighted by when frozen,
// and whether it was won.
type SettledChance struct {
	Percent int
	Won     bool
}

// AccuracyBand is the deals whose chance fell in one band of ten percent.
type AccuracyBand struct {
	From, To int
	Deals    int
	// MeanPercent and WonPercent are nil below the floor.
	MeanPercent, WonPercent *int
}

// AccuracyCalibration is every band that holds a deal, lowest first.
type AccuracyCalibration struct {
	Deals int
	Floor int
	Bands []AccuracyBand
}

// BinAccuracy groups settled deals by chance in bands of ten, the last band
// including 100. A band below floor states no mean and no share, because a
// share of a handful of deals reads as a rate it is not.
func BinAccuracy(settled []SettledChance, floor int) AccuracyCalibration {
	var count, won, sum [10]int
	for _, s := range settled {
		band := min(max(s.Percent, 0)/calibrationBandWidth, 9)
		count[band]++
		sum[band] += s.Percent
		if s.Won {
			won[band]++
		}
	}
	out := AccuracyCalibration{Deals: len(settled), Floor: floor, Bands: []AccuracyBand{}}
	for band := range count {
		if count[band] == 0 {
			continue
		}
		row := AccuracyBand{From: band * calibrationBandWidth, To: band*calibrationBandWidth + calibrationBandWidth - 1, Deals: count[band]}
		if band == 9 {
			row.To = 100
		}
		if count[band] >= floor {
			row.MeanPercent = new(int(math.Round(float64(sum[band]) / float64(count[band]))))
			row.WonPercent = new(int(math.Round(100 * float64(won[band]) / float64(count[band]))))
		}
		out.Bands = append(out.Bands, row)
	}
	return out
}

// AccuracyLanding sets what the forecast said a quarter would land at against what was
// won.
type AccuracyLanding struct {
	PeriodStart, PeriodEnd time.Time
	Currency               string
	SaidMinor, LandedMinor int64
	SaidOn                 time.Time
}

// Accuracy is the forecast's record so far.
type Accuracy struct {
	Calibration AccuracyCalibration
	Landings    []AccuracyLanding
}

// AccuracyToWire carries the accuracy across to the contract.
func AccuracyToWire(in Accuracy) crmcontracts.ForecastAccuracy {
	out := crmcontracts.ForecastAccuracy{
		Calibration: crmcontracts.ForecastCalibration{
			Deals: in.Calibration.Deals, Floor: in.Calibration.Floor, LeadDays: CalibrationLeadDays,
			Bands: make([]crmcontracts.ForecastCalibrationBand, 0, len(in.Calibration.Bands)),
		},
		Landings: make([]crmcontracts.ForecastAccuracyLanding, 0, len(in.Landings)),
	}
	for _, band := range in.Calibration.Bands {
		out.Calibration.Bands = append(out.Calibration.Bands, crmcontracts.ForecastCalibrationBand{
			FromPercent: band.From, ToPercent: band.To, Deals: band.Deals,
			MeanPercent: band.MeanPercent, WonPercent: band.WonPercent,
		})
	}
	for _, l := range in.Landings {
		out.Landings = append(out.Landings, crmcontracts.ForecastAccuracyLanding{
			PeriodStart: openapi_types.Date{Time: l.PeriodStart}, PeriodEnd: openapi_types.Date{Time: l.PeriodEnd},
			Currency: l.Currency, SaidMinor: l.SaidMinor, LandedMinor: l.LandedMinor,
			SaidOn: openapi_types.Date{Time: l.SaidOn},
		})
	}
	return out
}
