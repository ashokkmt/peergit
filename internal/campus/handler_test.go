package campus

import "testing"

func TestTenantAdminInputValidation(t *testing.T) {
	for _, slug := range []string{"robotics-club", "a1", "maker-space-2"} {
		if !validSlug(slug) {
			t.Errorf("valid slug %q rejected", slug)
		}
	}
	for _, slug := range []string{"", "-club", "club-", "two--hyphens", "UPPER", "not/slug", "x"} {
		if validSlug(slug) {
			t.Errorf("invalid slug %q accepted", slug)
		}
	}
	for _, role := range []string{"student", "faculty", "alumni_mentor", "moderator", "campus_admin"} {
		if !validRole(role) {
			t.Errorf("role %q rejected", role)
		}
	}
	if validExternal("campus_admin") || validOrganization("campus_admin") {
		t.Fatal("a campus role was accepted in an unrelated scope")
	}
	for _, id := range []string{"0197d900-72b8-7000-8000-000000000001", "ffffffff-ffff-7fff-8fff-ffffffffffff"} {
		if !validUUID(id) {
			t.Errorf("valid lowercase UUID %q rejected", id)
		}
	}
	for _, id := range []string{"", "../admin", "0197d90072b870008000000000000001", "0197D900-72B8-7000-8000-000000000001"} {
		if validUUID(id) {
			t.Errorf("invalid UUID %q accepted", id)
		}
	}
}
