package asten

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"golang.org/x/exp/slog"
)

type groupStats struct {
	*sync.RWMutex
	group *GroupSt
	valid bool

	totalTime     uint64
	effectiveTime uint64
	nsamples      uint64
}

func newGroupStats(g *GroupSt) *groupStats {
	gd := &groupStats{
		RWMutex:       &sync.RWMutex{},
		group:         g,
		valid:         false,
		totalTime:     0,
		effectiveTime: 0,
		nsamples:      0,
	}

	return gd
}

func (gs *groupStats) String() string {
	var b bytes.Buffer

	b.WriteString("[statistics]\n")
	b.WriteString(fmt.Sprintf("valid: %t\n", gs.valid))
	b.WriteString(fmt.Sprintf("totalTime: %s\n", time.Duration(gs.totalTime)))
	b.WriteString(fmt.Sprintf("effectiveTime: %s\n", time.Duration(gs.effectiveTime)))
	b.WriteString(fmt.Sprintf("nsamples: %d\n", gs.nsamples))

	return b.String()
}

func (s *groupStats) update() {
	if s.valid {
		return
	}

	s.valid = true

	s.totalTime = 0
	s.effectiveTime = 0
	s.nsamples = 0

	for spName := range s.group.profiles {
		subStats := s.group.profiles[spName].stats
		s.totalTime += subStats.totalTime
		s.effectiveTime += subStats.effectiveTime
		s.nsamples += subStats.nsamples
	}

	for spName := range s.group.profiles {
		subStats := s.group.profiles[spName].stats
		subStats.timeslice = float64(subStats.effectiveTime) / float64(s.effectiveTime)
		subStats.taken = float64(subStats.nsamples) / float64(s.nsamples)
	}
}

func (gs *groupStats) copy() *groupStats {
	return &groupStats{
		RWMutex:       &sync.RWMutex{},
		group:         gs.group,
		valid:         gs.valid,
		totalTime:     gs.totalTime,
		effectiveTime: gs.effectiveTime,
		nsamples:      gs.nsamples,
	}
}

type profileStats struct {
	*sync.RWMutex
	profile *ProfileSt
	valid   bool

	totalTime     uint64
	effectiveTime uint64
	meanTime      uint64
	nsamples      uint64
	timeslice     float64
	taken         float64

	samples []sample
}

func newProfileStats(p *ProfileSt) *profileStats {
	ps := &profileStats{
		RWMutex:       &sync.RWMutex{},
		profile:       p,
		valid:         true,
		totalTime:     0,
		effectiveTime: 0,
		meanTime:      0,
		nsamples:      0,
		timeslice:     0,
	}

	return ps
}

func (ps *profileStats) String() string {
	var b bytes.Buffer

	b.WriteString("[statistics]\n")
	b.WriteString(fmt.Sprintf("valid: %t\n", ps.valid))
	b.WriteString(fmt.Sprintf("totalTime: %s\n", time.Duration(ps.totalTime)))
	b.WriteString(fmt.Sprintf("effectiveTime: %s\n", time.Duration(ps.effectiveTime)))
	b.WriteString(fmt.Sprintf("meanTime: %s\n", time.Duration(ps.meanTime)))
	b.WriteString(fmt.Sprintf("nsamples: %d\n", ps.nsamples))
	b.WriteString(fmt.Sprintf("timeslice: %f\n", ps.timeslice))
	b.WriteString(fmt.Sprintf("taken: %f\n", ps.taken))

	return b.String()
}

func (ps *profileStats) copy() *profileStats {
	cps := &profileStats{
		RWMutex:       &sync.RWMutex{},
		profile:       nil,
		valid:         ps.valid,
		totalTime:     ps.totalTime,
		effectiveTime: ps.effectiveTime,
		meanTime:      ps.meanTime,
		nsamples:      ps.nsamples,
		timeslice:     ps.timeslice,
		taken:         ps.taken,
		samples:       ps.samples,
	}

	return cps
}

func (s *profileStats) invalidate() {
	s.valid = false

	pp := s.profile.parent
	if pp != nil {
		pp.stats.invalidate()
	}
}

func (s *profileStats) update() {
	if s.valid {
		return
	}

	s.valid = true

	if !s.profile.composite {
		if !s.profile.memory {
			logger.Error(
				"invalid profile statistics state: non composite memoryless statistics should always be valid",
				slog.String("profile", s.profile.getFullName()),
			)
			return
		}

		s.totalTime = 0
		s.effectiveTime = 0
		s.nsamples = uint64(len(s.samples))

		if s.nsamples == 0 {
			s.meanTime = 0
			s.timeslice = 0
			return
		}

		for _, sample := range s.samples {
			s.totalTime += sample.getDurationNano()
		}
		if s.nsamples < s.profile.nThreads {
			s.effectiveTime = s.totalTime / s.nsamples
		} else {
			s.effectiveTime = s.totalTime / s.profile.nThreads
		}

		s.meanTime = s.effectiveTime / s.nsamples
		return
	}

	s.totalTime = 0
	s.effectiveTime = 0

	for spName := range s.profile.subProfiles {
		subStats := s.profile.subProfiles[spName].stats
		s.totalTime += subStats.totalTime
		s.effectiveTime += subStats.effectiveTime
		s.nsamples += subStats.nsamples
	}

	s.meanTime = s.effectiveTime / s.nsamples

	for spName := range s.profile.subProfiles {
		subStats := s.profile.subProfiles[spName].stats
		subStats.timeslice = float64(subStats.effectiveTime) / float64(s.effectiveTime)
		subStats.taken = float64(subStats.nsamples) / float64(s.nsamples)
	}
}

func (s *profileStats) registerSample(sample sample) {
	s.invalidate()
	if s.profile.memory {
		s.samples = append(s.samples, sample)
		s.nsamples++
		return
	}

	duration := sample.getDurationNano()
	s.nsamples++
	s.totalTime += duration
	s.effectiveTime += duration / s.profile.nThreads
	s.meanTime = s.effectiveTime / s.nsamples
	s.valid = true
}

type sample struct {
	start time.Time
	end   time.Time
}

func newSample(start, end time.Time) sample {
	return sample{start: start, end: end}
}

func (s sample) getDurationNano() uint64 {
	return uint64(s.end.Sub(s.start).Nanoseconds())
}
