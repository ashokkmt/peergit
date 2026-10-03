package recruitment

import "testing"

func TestDifficultyOK(t *testing.T) {
	for _, value := range []string{"novice", "beginner", "intermediate", "advanced"} {
		if !difficultyOK(value) {
			t.Errorf("difficulty %q rejected", value)
		}
	}
	for _, value := range []string{"", "expert", "Beginner"} {
		if difficultyOK(value) {
			t.Errorf("difficulty %q accepted", value)
		}
	}
}

func TestUUIDOK(t *testing.T) {
	for _, value := range []string{"0192f64a-7b4e-7a10-8a4b-9f2e7b5d13ab", "00000000-0000-0000-0000-000000000000"} {
		if !uuidOK(value) {
			t.Errorf("UUID %q rejected", value)
		}
	}
	for _, value := range []string{"", "not-a-uuid", "0192f64a-7b4e-7a10-8a4b-9f2e7b5d13aB", "0192f64a7b4e7a108a4b9f2e7b5d13ab"} {
		if uuidOK(value) {
			t.Errorf("invalid UUID %q accepted", value)
		}
	}
}
