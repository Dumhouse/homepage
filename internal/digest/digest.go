// Package digest builds the weekly summary sent through the alert notifiers.
package digest

import (
	"fmt"
	"strings"
	"time"
)

type Counts struct {
	Hosts     int
	Services  int
	Projects  int
	Locations int
}

type CheckCounts struct {
	Up, Failing, Down, Recovering, Unknown, Disabled int
}

type Problem struct {
	Title   string
	State   string
	Message string
}

type Input struct {
	At          time.Time
	Counts      Counts
	Checks      CheckCounts
	Problems    []Problem
	Expiring    []string
	Maintenance int
}

// ShouldSend is false until the first run has stored a timestamp (so a fresh
// install does not send a digest immediately).
func ShouldSend(now time.Time, lastUnix int64, every time.Duration) bool {
	if lastUnix == 0 {
		return false
	}
	return now.Sub(time.Unix(lastUnix, 0)) >= every
}

func Subject() string { return "Home Services weekly digest" }

func Build(in Input) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Home Services — weekly digest (%s)\n\n", in.At.UTC().Format("2 Jan 2006"))
	fmt.Fprintf(&b, "%d hosts · %d services · %d projects · %d locations\n",
		in.Counts.Hosts, in.Counts.Services, in.Counts.Projects, in.Counts.Locations)
	fmt.Fprintf(&b, "Checks: %d up · %d failing · %d down · %d recovering · %d unknown",
		in.Checks.Up, in.Checks.Failing, in.Checks.Down, in.Checks.Recovering, in.Checks.Unknown)
	if in.Checks.Disabled > 0 {
		fmt.Fprintf(&b, " · %d paused", in.Checks.Disabled)
	}
	b.WriteString("\n")

	if len(in.Problems) == 0 {
		b.WriteString("\nNo problems right now.\n")
	} else {
		fmt.Fprintf(&b, "\nProblems (%d):\n", len(in.Problems))
		for _, p := range in.Problems {
			line := fmt.Sprintf("• %s — %s", p.Title, p.State)
			if p.Message != "" {
				line += ": " + p.Message
			}
			b.WriteString(line + "\n")
		}
	}
	if len(in.Expiring) > 0 {
		b.WriteString("\nExpiring soon:\n")
		for _, e := range in.Expiring {
			b.WriteString("• " + e + "\n")
		}
	}
	if in.Maintenance > 0 {
		fmt.Fprintf(&b, "\nMaintenance: %d window(s) active now.\n", in.Maintenance)
	}
	return b.String()
}
