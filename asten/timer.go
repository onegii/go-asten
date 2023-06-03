package asten

import "time"

// # Timer
//
// Represents a running timer.
// Its zero value has no meaning. A Timer should always be instantiated by
// calling either [GroupSt.StartTimer] or [ProfileSt.StartTimer].
type Timer struct {
	profile *ProfileSt
	conds   []string
	start   time.Time
	end     time.Time
}

// Stop is equivalent to calling:
//
//	t.StopAs([]string{default_condition_name})
//
// (see [SetDefaultConditionName]).
func (t *Timer) Stop() {
	t.end = time.Now()
	t.conds = []string{default_condition_name}
	t.profile.registerTimer(t)
}

// StopAs stops the timer and registers the sample in the profile that started
// the timer.
// Each element in conds corresponds to a sub-profile.
// For example:
//
//	t := Profile("p1").StartTimer()
//	// ...
//	t.StopAs("foo", "bar")
//
// Will have the sample be registered in the sub-profile:
//
//	p1
//	 └ foo
//	     └ bar
//
// If bar is not composite, or:
//
//	p1
//	 └ foo
//	     └ bar
//	         └ default_condition_name
//
// If bar is composite (see [SetDefaultConditionName]).
func (t *Timer) StopAs(conds ...string) {
	t.end = time.Now()
	t.conds = conds
	t.profile.registerTimer(t)
}
