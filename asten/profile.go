package asten

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"golang.org/x/exp/slog"
)

// # ProfileSt (Profile Struct)
//
// Represents a profile which groups and elaborates info obtained using timers.
// A profile can be:
//   - non-composite
//   - composite: it represent a collection of subprofiles.
//   - single-threaded
//   - multi-threaded: in this case the effective runtime of the profile will
//     simply be divided by the number of threads specified but capped by the number of
//     cores on the machine.
//   - memory full: it keeps memory of the start and the end of all recorded samples. This
//     avoids updating the statistics each time a sample is recirded.
//   - memoryless: when recording a sample statistics are updated and the sample discarded.
//
// Its zero value has no meaning and should not be used. A profile should always be instantiated
// either using [Profile], [GroupSt.Profile] or a custom builder [ProfileBuilder].
type ProfileSt struct {
	*sync.RWMutex
	name string

	parent *ProfileSt

	composite   bool
	builder     *ProfileBuilder
	subProfiles map[string]*ProfileSt

	nThreads uint64
	memory   bool
	stats    *profileStats
}

// Profile returns the sub-profile named pname belonging to profile p.
// If no such profile exists it is created using the profile builder (see
// [ProfileSt.Builder]).
func (p *ProfileSt) Profile(pname string) *ProfileSt {
	p.RLock()
	sp, ok := p.subProfiles[pname]
	p.RUnlock()

	if ok {
		return sp
	}

	sp = p.builder.NewProfile(pname)
	return sp
}

func (p *ProfileSt) addProfile(sp *ProfileSt) *ProfileSt {
	// adding a profile to a non composite one will cause it to be converted
	// samples registered while profile was not composite will be lost
	if !p.composite {
		logger.Warn("making profile composite, previous samples will be lost",
			slog.String("profile", p.getFullName()))
		p.unsafeMakeComposite()
	}

	// if subprofile was already declared return it, discarding the new one
	if tmp, ok := p.subProfiles[sp.name]; ok {
		logger.Warn("attempt to redeclare profile detected",
			slog.String("profile", sp.getFullName()), slog.String("function", "addprofile"))
		return tmp
	}

	// set p as parent of sp and add it to p's subprofiles
	sp.parent = p
	p.subProfiles[sp.name] = sp

	return sp
}

// MakeComposite transforms profile p from non-composite to composite. Any
// sample recorded while p was non-composite will be lost.
func (p *ProfileSt) MakeComposite() *ProfileSt {
	p.recursiveLock()
	defer p.recursiveUnlock()

	if p.composite {
		return p
	}
	p.composite = true
	p.subProfiles = make(map[string]*ProfileSt)
	p.stats.Unlock()
	p.stats = newProfileStats(p)
	p.stats.Lock()

	return p
}

func (p *ProfileSt) unsafeMakeComposite() *ProfileSt {
	if p.composite {
		return p
	}
	p.composite = true
	p.subProfiles = make(map[string]*ProfileSt)
	p.stats = newProfileStats(p)

	return p
}

// Builder returns a pointer to the builder used to generate new sub-profiles
// for profile p.
func (p *ProfileSt) Builder() *ProfileBuilder {
	p.RLock()
	if p.composite {
		defer p.RUnlock()
		return p.builder
	}
	p.Unlock()

	p.recursiveLock()
	defer p.recursiveUnlock()

	logger.Warn(
		"requested builder of non composite profile. Profile will be made composite, all previous samples will be lost",
		slog.String("profile", p.getFullName()),
	)
	p.unsafeMakeComposite()
	return p.builder
}

// SetBuilder sets the builder used by profile p to generate new profiles to pb.
func (p *ProfileSt) SetBuilder(pb *ProfileBuilder) {
	p.Lock()
	defer p.Unlock()

	pb.parentProfile = p
	p.builder = pb
}

// StartTimer starts and returns a [Timer] relative to profile p.
func (p *ProfileSt) StartTimer() *Timer {
	return &Timer{
		profile: p,
		start:   time.Now(),
	}
}

func (p *ProfileSt) registerTimer(t *Timer) {
	if len(t.conds) > 1 {
		cond := t.conds[0]
		t.conds = t.conds[1:]
		p.Profile(cond).registerTimer(t)
		return
	}

	p.Lock()

	cond := t.conds[0]
	if !p.composite {
		// if profile is not composite but a condition is specified then the
		// profile is made composite and the timer is passed to a new subprofile
		if cond != default_condition_name {

			logger.Warn("making profile composite, previous samples will be lost",
				slog.String("profile", p.getFullName()))
			p.unsafeMakeComposite()

			t.conds = []string{default_condition_name}
			p.Unlock()

			p.Profile(cond).registerTimer(t)
			return
		}

		p.stats.Lock()
		p.stats.registerSample(newSample(t.start, t.end))

		p.Unlock()
		p.stats.Unlock()
		return
	}
	p.Unlock()

	t.conds = []string{default_condition_name}

	p.Profile(cond).registerTimer(t)
}

func (p *ProfileSt) getFullName() string {
	names := []string{p.name}

	for p.parent != nil {
		p = p.parent
		names = append(names, p.name)
	}

	var b bytes.Buffer
	for i := len(names) - 1; i > 0; i-- {
		b.WriteString(names[i])
		b.WriteString(" -> ")
	}
	b.WriteString(names[0])
	return b.String()
}

func (p *ProfileSt) String() string {
	b := bytes.NewBufferString("")

	b.WriteString(fmt.Sprintf("[Profile %s]\n", p.name))
	if p.parent != nil {
		b.WriteString(fmt.Sprintf("parent: %s\n", p.parent.name))
	}

	s := strings.Replace("\t"+p.stats.String(), "\n", "\n\t", -1)
	s = s[:len(s)-1]
	b.WriteString(s)

	b.WriteString(fmt.Sprintf("memory: %t\n", p.memory))
	b.WriteString(fmt.Sprintf("threads: %d\n", p.nThreads))
	b.WriteString(fmt.Sprintf("composite: %t\n", p.composite))
	if p.composite {
		s := p.builder.String()
		s = strings.Replace("\t"+s, "\n", "\n\t", -1)
		s = s[:len(s)-1]
		b.WriteString(fmt.Sprintf("builder: \n%s\n", s))
		b.WriteString("sub profiles:\n")
		for _, subp := range p.subProfiles {
			s := strings.Replace("\t"+subp.String(), "\n", "\n\t", -1)
			s = s[:len(s)-1]
			b.WriteString(s)
		}
	}

	return b.String()
}

// Print generates and prints in a recursive manner tables containing info
// about the profile p and its sub-profiles.
func (p *ProfileSt) Print() {
	p.recursiveLock()
	cp := p.updateAndCopy()
	p.recursiveUnlock()

	headerFmt := color.New(color.FgYellow, color.Underline).SprintfFunc()

	if !cp.composite {
		tbl := table.New(
			"profile",
			"total runtime",
			"effective runtime",
			"mean runtime",
			"branch taken",
			"nsamples",
		)
		tbl.WithHeaderFormatter(headerFmt)
		tbl.AddRow(cp.getFullName(),
			time.Duration(cp.stats.totalTime),
			time.Duration(cp.stats.effectiveTime),
			time.Duration(cp.stats.meanTime),
			cp.stats.taken,
			cp.stats.nsamples)

		color.New(color.FgYellow).Add(color.Bold).Printf("\n\u24c5 Profile %s\n", cp.name)
		tbl.Print()
		return
	}

	tbl := table.New(
		"profile",
		"timeslice",
		"total runtime",
		"effective runtime",
		"mean runtime",
		"branch taken",
		"nsamples",
	)
	tbl.WithHeaderFormatter(headerFmt)

	for spName := range cp.subProfiles {
		sp := cp.subProfiles[spName]
		tbl.AddRow(sp.getFullName(),
			math.Floor(sp.stats.timeslice*1000)/1000,
			time.Duration(sp.stats.totalTime),
			time.Duration(sp.stats.effectiveTime),
			time.Duration(sp.stats.meanTime),
			sp.stats.taken,
			sp.stats.nsamples)
	}
	color.New(color.FgYellow).Add(color.Bold).Printf("\n\u24c5 Profile %s\n", cp.name)
	tbl.Print()

	for spName := range cp.subProfiles {
		sp := cp.subProfiles[spName]
		if sp.composite {
			sp.print()
		}
	}
}

// Equivalent to Print but does not generate copy or updates
func (cp *ProfileSt) print() {
	headerFmt := color.New(color.FgYellow, color.Underline).SprintfFunc()

	if !cp.composite {
		tbl := table.New(
			"profile",
			"total runtime",
			"effective runtime",
			"mean runtime",
			"branch taken",
			"nsamples",
		)
		tbl.WithHeaderFormatter(headerFmt)
		tbl.AddRow(cp.getFullName(),
			time.Duration(cp.stats.totalTime),
			time.Duration(cp.stats.effectiveTime),
			time.Duration(cp.stats.meanTime),
			cp.stats.taken,
			cp.stats.nsamples)

		color.New(color.FgYellow).Add(color.Bold).Printf("\n\u24c5 Profile %s\n", cp.name)
		tbl.Print()
		return
	}

	tbl := table.New(
		"profile",
		"timeslice",
		"total runtime",
		"effective runtime",
		"mean runtime",
		"branch taken",
		"nsamples",
	)
	tbl.WithHeaderFormatter(headerFmt)

	for spName := range cp.subProfiles {
		sp := cp.subProfiles[spName]
		tbl.AddRow(sp.getFullName(),
			math.Floor(sp.stats.timeslice*1000)/1000,
			time.Duration(sp.stats.totalTime),
			time.Duration(sp.stats.effectiveTime),
			time.Duration(sp.stats.meanTime),
			sp.stats.taken,
			sp.stats.nsamples)
	}
	color.New(color.FgYellow).Add(color.Bold).Printf("\n\u24c5 Profile %s\n", cp.name)
	tbl.Print()

	for spName := range cp.subProfiles {
		sp := cp.subProfiles[spName]
		if sp.composite {
			sp.print()
		}
	}
}

func (p *ProfileSt) copy() *ProfileSt {
	cp := &ProfileSt{
		RWMutex: &sync.RWMutex{},
		name:    p.name,

		composite: p.composite,
		builder:   p.builder.Copy(),
		nThreads:  p.nThreads,
		memory:    p.memory,
		stats:     p.stats.copy(),
	}

	cp.stats.profile = cp

	if !p.composite {
		return cp
	}

	cp.subProfiles = make(map[string]*ProfileSt)
	for subp := range p.subProfiles {
		cp.subProfiles[subp] = p.subProfiles[subp].copy()
		cp.subProfiles[subp].parent = cp
	}
	return cp
}

func (p *ProfileSt) recursiveLock() {
	p.Lock()
	p.stats.Lock()

	for spName := range p.subProfiles {
		p.subProfiles[spName].Lock()
		p.subProfiles[spName].stats.Lock()
	}
}

func (p *ProfileSt) recursiveUnlock() {
	for spName := range p.subProfiles {
		p.subProfiles[spName].Unlock()
		p.subProfiles[spName].stats.Unlock()
	}
	p.Unlock()
	p.stats.Unlock()
}

func (p *ProfileSt) recursiveRLock() {
	p.RLock()
	p.stats.RLock()

	for spName := range p.subProfiles {
		p.subProfiles[spName].RLock()
		p.subProfiles[spName].stats.RLock()
	}
}

func (p *ProfileSt) recursiveRUnlock() {
	for spName := range p.subProfiles {
		p.subProfiles[spName].RUnlock()
		p.subProfiles[spName].stats.RUnlock()
	}
	p.RUnlock()
	p.stats.RUnlock()
}

func (p *ProfileSt) update() {
	for spName := range p.subProfiles {
		sp := p.subProfiles[spName]
		sp.update()
	}
	p.stats.update()
}

func (p *ProfileSt) updateAndCopy() *ProfileSt {
	p.update()
	cp := p.copy()

	return cp
}

// # ProfileBuilder
//
// ProfileBuilder implements a builder pattern to generate new profiles.
// A ProfileBuilder can have either a parent profile or a parent group (which
// can be set using [ProfileBuilder.WithParentGroup] or
// [ProfileBuilder.WithParentProfile]) that affect the behaviour of
// [ProfileBuilder.NewProfile].
// Its zero value has no particular meaning and should not be used.
// A ProfileBuilder should always be instantiated using [NewProfileBuilder].
type ProfileBuilder struct {
	parentGroup   *GroupSt
	parentProfile *ProfileSt
	composite     bool
	nThreads      uint64
	memory        bool
}

func (pb ProfileBuilder) String() string {
	b := bytes.NewBufferString("")

	if pb.parentGroup != nil {
		b.WriteString(fmt.Sprintf("builder for group: %s\n", pb.parentGroup.name))
	} else {
		b.WriteString("groupless builder\n")
	}
	if pb.parentProfile != nil {
		b.WriteString(fmt.Sprintf("builder for profile: %s\n", pb.parentProfile.name))
	} else {
		b.WriteString("parentless builder\n")
	}
	b.WriteString(fmt.Sprintf("composite: %t\n", pb.composite))
	b.WriteString(fmt.Sprintf("memory: %t\n", pb.memory))
	b.WriteString(fmt.Sprintf("threads: %d\n", pb.nThreads))

	return b.String()
}

// NewProfileBuilder returns a [ProfileBuilder] which will generate profiles
// that are:
//   - non-composite
//   - memoryless
//   - sigle-threaded
func NewProfileBuilder() *ProfileBuilder {
	return &ProfileBuilder{
		parentGroup:   nil,
		parentProfile: nil,
		composite:     false,
		memory:        false,
		nThreads:      1,
	}
}

// NewProfile generates a new profile whose characteristics are based on pb's
// state.
// If pb is the builder for a profile or a group, the generated profile
// is automatically added to it.
// The generated profile will have a builder with the same characteristics as
// pb except it will always be non-composite.
func (pb *ProfileBuilder) NewProfile(pname string) *ProfileSt {
	p := &ProfileSt{
		RWMutex:   &sync.RWMutex{},
		name:      pname,
		parent:    pb.parentProfile,
		composite: pb.composite,
		memory:    pb.memory,
		nThreads:  pb.nThreads,
	}

	p.builder = pb.Copy().RemoveComposition().WithParentProfile(p)

	if p.composite {
		p.subProfiles = make(map[string]*ProfileSt)
	}

	p.stats = newProfileStats(p)

	// assign o to profile or group
	if pb.parentProfile != nil {
		pb.parentProfile.Lock()
		p = pb.parentProfile.addProfile(p)
		pb.parentProfile.Unlock()
	} else if pb.parentGroup != nil {
		pb.parentGroup.addProfile(p)
	}

	return p
}

// Copy generates and returns a deep copy of pb.
func (pb ProfileBuilder) Copy() *ProfileBuilder {
	cpb := &ProfileBuilder{
		parentProfile: pb.parentProfile,
		composite:     pb.composite,
		memory:        pb.memory,
		nThreads:      pb.nThreads,
	}
	return cpb
}

// WithParentGroup modifies and returns pb, setting its parent group to g.
// Any profile generated calling [NewProfile] will be added to the profiles
// of group g.
// If pb had a parent profile, it will be deleted.
func (pb *ProfileBuilder) WithParentGroup(g *GroupSt) *ProfileBuilder {
	pb.parentGroup = g
	pb.parentProfile = nil
	return pb
}

// WithParentProfile modifies and returns pb, setting its parent profile to p.
// Any profile generated calling [NewProfile] will be added to the sub-profiles
// of profile p.
// If pb had a parent group, it will be deleted.
func (pb *ProfileBuilder) WithParentProfile(p *ProfileSt) *ProfileBuilder {
	pb.parentProfile = p
	pb.parentGroup = nil
	return pb
}

// AddComposition modifies and returns pb, making any new profile generated
// by calling [ProfileBuilder.NewProfile] a composite profile.
func (pb *ProfileBuilder) AddComposition() *ProfileBuilder {
	pb.composite = true
	return pb
}

// RemoveComposition modifies and returns pb, making any new profile generated
// by calling [ProfileBuilder.NewProfile] a non-composite profile.
func (pb *ProfileBuilder) RemoveComposition() *ProfileBuilder {
	pb.composite = false
	return pb
}

// AddMemory modifies and returns pb, making any new profile generated
// by calling [ProfileBuilder.NewProfile] a memory full profile.
func (pb *ProfileBuilder) AddMemory() *ProfileBuilder {
	pb.memory = true
	return pb
}

// RemoveMemory modifies and returns pb, making any new profile generated
// by calling [ProfileBuilder.NewProfile] a memoryless profile.
func (pb *ProfileBuilder) RemoveMemory() *ProfileBuilder {
	pb.memory = false
	return pb
}

// AddMultiThreading modifies and returns pb, making any new profile generated
// by calling [ProfileBuilder.NewProfile] a multi-threaded profile with n threads,
// where n is the number of cores (see [SetCoresNumber]).
func (pb *ProfileBuilder) AddMultiThreading() *ProfileBuilder {
	pb.nThreads = cores
	return pb
}

// RemoveMultiThreading modifies and returns pb, making any new profile generated
// by calling [ProfileBuilder.NewProfile] a single-threaded profile.
func (pb *ProfileBuilder) RemoveMultiThreading() *ProfileBuilder {
	pb.nThreads = 1
	return pb
}

// WithNThreads modifies and returns pb, making any new profile generated
// by calling [ProfileBuilder.NewProfile] a multi-threaded profile with n threads.
// If n > number of cores (see [SetCoresNumber]) then n is set to the number of cores.
func (pb *ProfileBuilder) WithNThreads(n uint64) *ProfileBuilder {
	if n <= 0 {
		logger.Error("number of threads must be > 0, setting value to 1")
		n = 1
	}

	if n > cores {
		n = cores
	}

	pb.nThreads = n
	return pb
}

// WithNThreads is equivalent to [ProfileBuilder.WithNThreads] but no check is done
// on n.
func (pb *ProfileBuilder) WithNCores(n uint64) *ProfileBuilder {
	if n <= 0 {
		logger.Error("number of threads must be > 0, setting value to 1")
		n = 1
	}

	pb.nThreads = n
	return pb
}
