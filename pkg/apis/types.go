package apis

type JobType string

const (
	ScaleUp   JobType = "scale-up"
	ScaleDown JobType = "scale-down"
)

type ScheduleJobs struct {
	Jobs []Job `json:"scheduleJobs"`
}

type Job struct {
	//
	Type       JobType `json:"type"`
	Schedule   string  `json:"schedule"`
	TargetSize int32   `json:"targetSize"`
}
