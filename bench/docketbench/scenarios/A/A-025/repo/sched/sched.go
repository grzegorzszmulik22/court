package sched

// Job is a unit of scheduled work.
type Job struct {
	Name     string
	EverySec int
	LastRun  int64
}

// Due reports whether the job should run at t.
func Due(j Job, t int64) bool {
	return t-j.LastRun >= int64(j.EverySec)
}

// Next returns the next run time.
func Next(j Job) int64 { return j.LastRun + int64(j.EverySec) }
