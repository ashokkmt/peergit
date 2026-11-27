package academic

import "testing"

func TestSafeCSVPrefixesSpreadsheetFormulas(t *testing.T) {
	for _, input := range []string{"=1+1", " +SUM(A1:A2)", "-2+3", "@SUM(A1)", "\t=cmd"} {
		got := safeCSV(input)
		if got == input || got[0] != '\'' {
			t.Errorf("safeCSV(%q) = %q, want a quoted literal", input, got)
		}
	}
	for _, input := range []string{"PeerGit", "student@example.edu", "2026-11-27"} {
		if got := safeCSV(input); got != input {
			t.Errorf("safeCSV(%q) = %q, want unchanged", input, got)
		}
	}
}
