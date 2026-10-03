package project

import "testing"

func TestCreateProjectRequestValidationAndNormalization(t *testing.T) {
	input := createProjectRequest{Slug: "  Campus-Robotics ", Title: " Robot team ", Summary: " Build small robots ", Type: "side_project", Skills: []string{" Go ", "CAD"}}
	if err := input.Validate(); err != nil {
		t.Fatalf("valid project rejected: %v", err)
	}
	if input.Slug != "campus-robotics" || input.Title != "Robot team" || input.Visibility != "campus" || input.Lifecycle != "draft" {
		t.Fatalf("defaults/normalization not applied: %+v", input)
	}
	if input.Skills[0] != "Go" {
		t.Fatalf("skill not normalized: %#v", input.Skills)
	}
}

func TestProjectSkillSlugsAndDeduplication(t *testing.T) {
	input := createProjectRequest{Slug: "robotics", Title: "Robotics", Summary: "Build robots", Type: "side_project", Skills: []string{"C++", "C++", "Node.js"}}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := skillSlug(input.Skills[0]); got != "c-plus-plus" {
		t.Fatalf("C++ slug=%q", got)
	}
	if len(input.Skills) != 2 {
		t.Fatalf("duplicate skills were not removed: %#v", input.Skills)
	}
}

func TestCreateProjectRequestRejectsUnsupportedOrOversizedFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*createProjectRequest)
	}{
		{"slug", func(v *createProjectRequest) { v.Slug = "bad--slug" }},
		{"visibility", func(v *createProjectRequest) { v.Visibility = "world" }},
		{"lifecycle", func(v *createProjectRequest) { v.Lifecycle = "deleted" }},
		{"type", func(v *createProjectRequest) { v.Type = "other" }},
		{"too many skills", func(v *createProjectRequest) { v.Skills = make([]string, 21) }},
		{"too many media", func(v *createProjectRequest) { v.MediaIDs = make([]string, 11) }},
		{"empty title", func(v *createProjectRequest) { v.Title = "  " }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := createProjectRequest{Slug: "robotics", Title: "Robotics", Summary: "Build robots", Type: "side_project"}
			tt.mutate(&v)
			if err := v.Validate(); err == nil {
				t.Fatal("invalid project was accepted")
			}
		})
	}
}

func TestProjectEnumerations(t *testing.T) {
	for _, v := range []string{"private", "campus", "public"} {
		if !visibilityOK(v) {
			t.Errorf("visibility %q rejected", v)
		}
	}
	for _, v := range []string{"draft", "active", "on_hold", "completed", "archived"} {
		if !lifecycleOK(v) {
			t.Errorf("lifecycle %q rejected", v)
		}
	}
	for _, v := range []string{"novice", "beginner", "intermediate", "advanced"} {
		if !difficultyOK(v) {
			t.Errorf("difficulty %q rejected", v)
		}
	}
	if visibilityOK("cross-campus") || lifecycleOK("recruiting") || difficultyOK("expert-only") {
		t.Fatal("unsupported enum was accepted")
	}
}
