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

var (
	// ggroups (global groups) is a map containing all project's groups
	ggroups = make(map[string]*GroupSt)
	// ggLock manages access to ggroups
	ggLock sync.RWMutex
)

// # GroupSt (Group Struct)
//
// Represents a group which is a collection of profiles whose statistics are
// to be compared. It is designed to be thread safe.
// Its zero value has no meaning and should not be used.
type GroupSt struct {
	*sync.RWMutex // manages concurrent access
	name          string
	stats         *groupStats

	builder  *ProfileBuilder
	profiles map[string]*ProfileSt
}

// Group returns the group with name: gname. If a group called gname exists
// then it will be returned otherwise a new group is created using a
// [NewProfileBuilder] and returned.
func Group(gname string) *GroupSt {
	// check if group already exists
	ggLock.RLock()
	g, ok := ggroups[gname]
	ggLock.RUnlock()

	// if found return it
	if ok {
		return g
	}

	// otherwise create it
	return newGroup(gname)
}

func newGroup(gname string) *GroupSt {
	// check that group does not already exist
	ggLock.Lock()
	defer ggLock.Unlock()

	if g, ok := ggroups[gname]; ok {
		logger.Warn("attempt to redeclare group detected",
			slog.String("group", gname))
		return g
	}

	// create group
	g := &GroupSt{
		RWMutex:  &sync.RWMutex{},
		name:     gname,
		profiles: make(map[string]*ProfileSt),
	}
	g.builder = NewProfileBuilder().WithParentGroup(g)
	g.stats = newGroupStats(g)

	// register group
	ggroups[gname] = g

	return g
}

// Profile returns the profile named pname belonging to group g.
// If no such profile exists it is created using the group builder (see
// [GroupSt.Builder]).
func (g *GroupSt) Profile(pname string) *ProfileSt {
	g.Lock()
	defer g.Unlock()

	p, ok := g.profiles[pname]

	if ok {
		return p
	}

	p = g.builder.NewProfile(pname)

	return p
}

// Profile is equivalent to calling [GroupSt.Profile] on the group with the
// deafult condition name (see [SetDefaultConditionName]), i.e., it is equivalent to:
//
//	Group(default_condition_name).Profile(pname)
func Profile(pname string) *ProfileSt {
	return Group(default_condition_name).Profile(pname)
}

func (g *GroupSt) addProfile(p *ProfileSt) *ProfileSt {
	// if subprofile was already declared return it, discarding the new one
	if tmp, ok := g.profiles[p.name]; ok {
		logger.Warn("attempt to redeclare profile detected",
			slog.String("profile", p.getFullName()))
		return tmp
	}

	p.parent = nil
	g.profiles[p.name] = p

	return p
}

// StartTimer is equivalent to calling:
//
//	g.Profile(default_condition_name).StartTimer()
//
// (see [SetDefaultConditionName]).
func (g *GroupSt) StartTimer() *Timer {
	return g.Profile(default_condition_name).StartTimer()
}

// Builder returns a pointer to the builder used to generate new profiles
// for group g.
func (g *GroupSt) Builder() *ProfileBuilder {
	return g.builder
}

// SetBuilder sets the builder used by group g to generate new profiles to pb.
func (g *GroupSt) SetBuilder(pb *ProfileBuilder) {
	pb.parentProfile = nil
	g.builder = pb
}

func (g *GroupSt) String() string {
	b := bytes.NewBufferString("")

	b.WriteString(fmt.Sprintf("[Group %s]\n", g.name))

	s := g.builder.String()
	s = strings.Replace("\t"+s, "\n", "\n\t", -1)
	s = s[:len(s)-1]

	b.WriteString(fmt.Sprintf("builder: \n%s\n", s))

	s = strings.Replace("\t"+g.stats.String(), "\n", "\n\t", -1)
	s = s[:len(s)-1]
	b.WriteString(s)

	b.WriteString("profiles:\n")
	for _, subp := range g.profiles {
		s = strings.Replace("\t"+subp.String(), "\n", "\n\t", -1)
		s = s[:len(s)-1]
		b.WriteString(s)
	}

	return b.String()
}

// Print generates and prints in a recursive manner tables containing info
// about the group g and its profiles.
func (g *GroupSt) Print() {
	g.recursiveLock()
	cg := g.updateAndCopy()
	g.recursiveUnlock()

	headerFmt := color.New(color.FgGreen, color.Underline).SprintfFunc()

	tbl := table.New(
		"group",
		"profile",
		"timeslice",
		"total runtime",
		"effective runtime",
		"mean runtime",
		"branch taken",
		"nsamples",
	)
	tbl.WithHeaderFormatter(headerFmt)

	for spName := range cg.profiles {
		sp := cg.profiles[spName]
		tbl.AddRow(
			g.name,
			sp.getFullName(),
			math.Floor(sp.stats.timeslice*1000)/1000,
			time.Duration(sp.stats.totalTime),
			time.Duration(sp.stats.effectiveTime),
			time.Duration(sp.stats.meanTime),
			sp.stats.taken,
			sp.stats.nsamples)
	}
	color.New(color.FgGreen).Add(color.Bold).Printf("\n\u24bc Group %s\n", g.name)
	tbl.Print()

	for profileName := range cg.profiles {
		p := cg.profiles[profileName]
		p.print()
	}
}

// PrintGroups generates and prints in a recursive manner tables containing info
// regarding all the declared groups and their profiles.
func PrintGroups() {
	ggLock.Lock()
	defer ggLock.Unlock()

	headerFmt := color.New(color.FgWhite, color.Underline).SprintfFunc()

	tbl := table.New(
		"group",
		"total runtime",
		"effective runtime",
		"nsamples",
	)
	tbl.WithHeaderFormatter(headerFmt)

	cgs := make(map[string]*GroupSt)
	for gName := range ggroups {
		g := ggroups[gName]
		g.recursiveLock()
		cgs[gName] = g.updateAndCopy()
		g.recursiveUnlock()
	}

	for profileName := range cgs {
		p := cgs[profileName]
		tbl.AddRow(
			p.name,
			time.Duration(p.stats.totalTime),
			time.Duration(p.stats.effectiveTime),
			p.stats.nsamples)
	}
	color.New(color.FgWhite).Add(color.Bold).Printf("\n\uf111 Groups\n")
	tbl.Print()

	for gName := range cgs {
		g := cgs[gName]
		g.Print()
	}
}

func (g *GroupSt) copy() *GroupSt {
	cp := &GroupSt{
		RWMutex:  &sync.RWMutex{},
		name:     g.name,
		stats:    g.stats.copy(),
		builder:  g.builder.Copy(),
		profiles: make(map[string]*ProfileSt),
	}

	for pname := range g.profiles {
		cp.profiles[pname] = g.profiles[pname].copy()
	}

	return cp
}

func (g *GroupSt) updateAndCopy() *GroupSt {
	g.update()
	return g.copy()
}

func (g *GroupSt) recursiveLock() {
	g.Lock()
	g.stats.Lock()
	for pname := range g.profiles {
		g.profiles[pname].recursiveLock()
	}
}

func (g *GroupSt) recursiveUnlock() {
	for pname := range g.profiles {
		g.profiles[pname].recursiveUnlock()
	}
	g.stats.Unlock()
	g.Unlock()
}

func (g *GroupSt) update() {
	for pname := range g.profiles {
		g.profiles[pname].update()
	}
	g.stats.update()
}
