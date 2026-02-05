package model

import (
	"net/netip"
	"time"
)

type DomainResult struct {
	Domain     string
	Best       CandidateStat
	Candidates []CandidateStat
	Err        error
}

type CandidateStat struct {
	IP          netip.Addr
	Successes   int
	Failures    int
	Samples     []time.Duration
	P50         time.Duration
	P95         time.Duration
	JitterStd   time.Duration
	LastError   string
	ResolvedVia string
}

func (c CandidateStat) Attempts() int { return c.Successes + c.Failures }

func (c CandidateStat) SuccessRate() float64 {
	if c.Attempts() == 0 {
		return 0
	}
	return float64(c.Successes) / float64(c.Attempts())
}

