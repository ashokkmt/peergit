package repository

import (
	"strings"
	"testing"
)

func TestImportedProjectSummaryIsNonemptyAndBounded(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{name: "empty description uses repository name", description: "", want: "Imported from GitHub: student/robotics"},
		{name: "whitespace description uses repository name", description: " \t\n", want: "Imported from GitHub: student/robotics"},
		{name: "description is trimmed", description: "  Robotics project  ", want: "Robotics project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := importedProjectSummary(tt.description, "student/robotics"); got != tt.want {
				t.Fatalf("importedProjectSummary() = %q, want %q", got, tt.want)
			}
		})
	}

	longDescription := strings.Repeat("x", 501)
	if got := importedProjectSummary(longDescription, "student/robotics"); len(got) != 500 {
		t.Fatalf("importedProjectSummary() length = %d, want 500", len(got))
	}
}
