package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitUser(t *testing.T) {
	user := "<@USERID|userName"
	id, name := SplitUser(user)
	assert.Equal(t, "USERID", id)
	assert.Equal(t, "userName", name)
}

func TestSecondsToHuman(t *testing.T) {
	testCases := []struct {
		output  string
		seconds int
	}{
		{"00:03", 180},
		{"00:04", 240},
		{"01:00", 3600},
		{"01:03", 3780},
		{"01:03", 3782},
	}
	for _, tt := range testCases {
		text := SecondsToHuman(tt.seconds)
		assert.Equal(t, tt.output, text)
	}

}

func TestSplitTimeTalbeCommand(t *testing.T) {
	testCases := []struct {
		command  string
		users    string
		weekdays string
		time     string
	}{
		{"@anatoliy on friday at 1pm", "@anatoliy", "friday", "1pm"},
		{"<@UB9AE7CL9|fedorenko.tolik> on monday at 5pm", "<@UB9AE7CL9|fedorenko.tolik>", "monday", "5pm"},
		{"@anatoliy @erik @alex on friday tuesday monday wednesday at 3pm", "@anatoliy @erik @alex", "friday tuesday monday wednesday", "3pm"},
	}
	for _, tt := range testCases {
		users, weekdays, deadline := SplitTimeTalbeCommand(tt.command, " on ", " at ")
		assert.Equal(t, tt.users, users)
		assert.Equal(t, tt.weekdays, weekdays)
		assert.Equal(t, tt.time, deadline)
	}

	testCasesRus := []struct {
		command  string
		users    string
		weekdays string
		time     string
	}{
		{"@anatoliy по пятницам в 1pm", "@anatoliy", "пятницам", "1pm"},
		{"@anatoliy @erik @alex по понедельникам пятницам вторникам в 3pm", "@anatoliy @erik @alex", "понедельникам пятницам вторникам", "3pm"},
	}
	for _, tt := range testCasesRus {
		users, weekdays, deadline := SplitTimeTalbeCommand(tt.command, " по ", " в ")
		assert.Equal(t, tt.users, users)
		assert.Equal(t, tt.weekdays, weekdays)
		assert.Equal(t, tt.time, deadline)
	}
}

func TestFormatTime(t *testing.T) {
	testCases := []struct {
		timeString string
		hour       int
		minute     int
		err        error
	}{
		{"10:00", 10, 0, nil},
		{"11:20", 11, 20, nil},
		{"25:20", 0, 0, errors.New("time format error")},
		{"25:20:30", 0, 0, errors.New("time format error")},
		{"shit:fuck", 0, 0, errors.New("time format error")},
		{"10:fuck", 0, 0, errors.New("time format error")},
	}
	for _, tt := range testCases {
		h, m, err := FormatTime(tt.timeString)
		assert.Equal(t, tt.hour, h)
		assert.Equal(t, tt.minute, m)
		assert.Equal(t, tt.err, err)
	}
}
