// Package asten provides functionalities for runtime performance evaluation.
//
// Statistics are organized in Groups. Each group contains one or more Profiles.
// Each Profile may be composite, i.e., a collection of sub-profiles.
// An example structure may be:
//
//	 Group 1
//	  ├ Profile 1.1
//	  └ Profile 1.2
//		   ├ Sub-Profile 1.2.1
//		   └ Sub-Profile 1.2.2
//
//	 Group 2
//	  └ Profile 2.1
//	    └ Sub-Profile 2.1.1
//	      └ Sub-Profile 2.1.1.1
//
// Statistics are presented in a tabular manner.
package asten

import (
	"os"
	"runtime"

	"golang.org/x/exp/slog"
)

var default_condition_name = "base"

func init() {
	cores = uint64(runtime.NumCPU())

	logLevel = new(slog.LevelVar)
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	logger = slog.New(h)
}

var (
	cores    uint64
	logger   *slog.Logger
	logLevel *slog.LevelVar
)

// SetLogger set the logger used by asten.
// [SetLogLevel] will not be enforced if a custom logger is used.
func SetLogger(newlogger *slog.Logger) {
	logger = newlogger
}

// SetLogLevel sets the level for asten messages unless [SetLogger] has been called.
// The default log level is the zero value of [slog.LevelVar].
func SetLogLevel(level slog.Level) {
	logLevel.Set(level)
}

// SetDefaultConditionName sets the name given to groups and profiles when a name is not specified.
// The default value is "base".
func SetDefaultConditionName(name string) {
	default_condition_name = name
}

// SetCoresNumber sets the number of cores available when calculating statistics.
// Default value is initialized using [runtime.NumCPU].
func SetCoresNumber(n uint64) {
	if n > 0 {
		cores = n
	} else {
		logger.Error("invalid cores number",
			slog.Uint64("n", n))
	}
}
