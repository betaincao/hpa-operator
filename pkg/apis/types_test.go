package apis

import (
	"encoding/json"
	"fmt"
	"testing"
)

func Test_test(t *testing.T) {
	scheduleJobs := ScheduleJobs{
		Jobs: []Job{
			{
				Type:       ScaleUp,
				Schedule:   "0 0 */10 * * *",
				TargetSize: 10,
			},
			{
				Type:       ScaleDown,
				Schedule:   "0 0 */15 * * *",
				TargetSize: 5,
			},
		},
	}
	scheduleJobsBytes, err := json.Marshal(scheduleJobs)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(string(scheduleJobsBytes))
}
