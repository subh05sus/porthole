package procfmt

import (
	"fmt"
	"strconv"
	"strings"
)

// StatInfo is what we need from /proc/[pid]/stat: the process name and its
// start time in clock ticks since boot (field 22), used only to detect PID
// reuse between scan time and kill time (see scan.Service.StartTime).
type StatInfo struct {
	PID            int
	Comm           string
	State          byte
	StartTimeTicks uint64
}

// ParseStat parses the contents of /proc/[pid]/stat. The comm field is
// parenthesized and may itself contain spaces or parentheses (a process can
// name itself almost anything via prctl/argv[0]), so the field is located
// by the first '(' and the LAST ')' in the line — the same trick the
// kernel's own tools use — rather than naive whitespace splitting.
func ParseStat(data []byte) (StatInfo, error) {
	s := string(data)

	open := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if open < 0 || end < 0 || end < open {
		return StatInfo{}, fmt.Errorf("procfmt: no comm field found in stat line")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(s[:open]))
	if err != nil {
		return StatInfo{}, fmt.Errorf("procfmt: bad pid field: %w", err)
	}
	comm := s[open+1 : end]

	rest := strings.Fields(s[end+1:])
	// state=0 ppid=1 pgrp=2 session=3 tty_nr=4 tpgid=5 flags=6 minflt=7
	// cminflt=8 majflt=9 cmajflt=10 utime=11 stime=12 cutime=13 cstime=14
	// priority=15 nice=16 num_threads=17 itrealvalue=18 starttime=19
	const startTimeIdx = 19
	if len(rest) <= startTimeIdx {
		return StatInfo{}, fmt.Errorf("procfmt: stat line too short: %d fields after comm", len(rest))
	}

	var state byte
	if len(rest[0]) > 0 {
		state = rest[0][0]
	}

	startTime, err := strconv.ParseUint(rest[startTimeIdx], 10, 64)
	if err != nil {
		return StatInfo{}, fmt.Errorf("procfmt: bad starttime field: %w", err)
	}

	return StatInfo{PID: pid, Comm: comm, State: state, StartTimeTicks: startTime}, nil
}
