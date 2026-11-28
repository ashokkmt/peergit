package academic

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"peergit/internal/identity"
	"peergit/internal/platform/errormanager"
)

type Handler struct {
	pool *pgxpool.Pool
	auth *identity.Handler
	err  *errormanager.Manager
}

func NewHandler(pool *pgxpool.Pool, auth *identity.Handler, errors *errormanager.Manager) *Handler {
	return &Handler{pool: pool, auth: auth, err: errors}
}

func (h *Handler) Register(r chi.Router) {
	r.Group(func(api chi.Router) {
		api.Use(h.auth.Middleware)
		api.With(h.auth.RequireCSRF).Post("/me/faculty-verification-requests", h.requestFacultyVerification)
		api.Get("/me/faculty-verification-requests", h.myFacultyVerification)
		api.Get("/admin/faculty-verification-requests", h.facultyQueue)
		api.Get("/admin/faculty-profiles", h.facultyProfiles)
		api.With(h.auth.RequireCSRF).Post("/admin/faculty-verification-requests/{id}/decision", h.decideFacultyVerification)
		api.With(h.auth.RequireCSRF).Post("/admin/faculty/{userID}/status", h.setFacultyStatus)
		api.Get("/admin/mentor-profiles", h.mentorQueue)
		api.With(h.auth.RequireCSRF).Post("/admin/mentor-profiles/{id}/decision", h.decideMentorProfile)
		api.Get("/me/mentor-profile", h.getMentorProfile)
		api.With(h.auth.RequireCSRF).Put("/me/mentor-profile", h.saveMentorProfile)
		api.Get("/mentors", h.listMentors)
		api.Get("/me/mentorship-requests", h.listMentorshipRequests)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/mentorship-requests", h.requestMentorship)
		api.With(h.auth.RequireCSRF).Post("/mentorship-requests/{id}/decision", h.decideMentorship)
		api.With(h.auth.RequireCSRF).Post("/mentorship-requests/{id}/end", h.endMentorship)
		api.Get("/mentorship/conversations/{id}/messages", h.mentorshipMessages)
		api.With(h.auth.RequireCSRF).Post("/mentorship/conversations/{id}/messages", h.sendMentorshipMessage)
		api.With(h.auth.RequireCSRF).Post("/mentorship/conversations/{id}/block", h.blockMentorshipParticipant)
		api.With(h.auth.RequireCSRF).Post("/mentorship/conversations/{id}/reports", h.reportMentorshipMessage)
		api.With(h.auth.RequireCSRF).Post("/academic/terms", h.createTerm)
		api.Get("/academic/terms", h.listTerms)
		api.With(h.auth.RequireCSRF).Post("/academic/courses", h.createCourse)
		api.Get("/academic/courses", h.listCourses)
		api.Get("/academic/faculty", h.listFaculty)
		api.Get("/academic/projects", h.listProjects)
		api.With(h.auth.RequireCSRF).Post("/academic/courses/{id}/cohorts", h.createCohort)
		api.Get("/academic/cohorts", h.listCohorts)
		api.With(h.auth.RequireCSRF).Post("/academic/cohorts/{id}/enrollments", h.enrollStudent)
		api.With(h.auth.RequireCSRF).Post("/academic/cohorts/{id}/assignments", h.createAssignment)
		api.Get("/academic/assignments", h.listAssignments)
		api.With(h.auth.RequireCSRF).Post("/academic/projects/{id}/endorsements", h.endorseProject)
		api.Get("/academic/projects/{id}/endorsements", h.listEndorsements)
		api.With(h.auth.RequireCSRF).Post("/academic/endorsements/{id}/revoke", h.revokeEndorsement)
		api.Get("/academic/calls", h.listCalls)
		api.Get("/academic/program-cohorts", h.listProgramCohorts)
		api.With(h.auth.RequireCSRF).Post("/academic/calls/{id}/close", h.closeCall)
		api.Get("/academic/calls/{id}/applications", h.listApplications)
		api.With(h.auth.RequireCSRF).Post("/academic/calls", h.createCall)
		api.With(h.auth.RequireCSRF).Post("/academic/calls/{id}/applications", h.applyToCall)
		api.With(h.auth.RequireCSRF).Post("/academic/calls/{id}/reviewers", h.assignReviewer)
		api.With(h.auth.RequireCSRF).Post("/academic/applications/{id}/review", h.reviewApplication)
		api.With(h.auth.RequireCSRF).Post("/academic/applications/{id}/decision", h.decideApplication)
		api.With(h.auth.RequireCSRF).Post("/academic/calls/{id}/cohorts", h.createProgramCohort)
		api.With(h.auth.RequireCSRF).Post("/academic/cohorts/{id}/milestones", h.createMilestone)
		api.With(h.auth.RequireCSRF).Post("/academic/cohorts/{id}/outcomes", h.saveOutcome)
		api.With(h.auth.RequireCSRF).Post("/academic/exports", h.createExport)
		api.Get("/academic/exports/{id}", h.downloadExport)
		api.Get("/academic/digest", h.academicDigest)
		api.With(h.auth.RequireCSRF).Put("/me/academic-digest-preferences", h.saveDigestPreferences)
		h.registerPBL(api)
	})
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return s, false
	}
	if s.CollegeID == "" && s.AccountType == "external" {
		scope := r.URL.Query().Get("college_id")
		var selected string
		var count int
		err := h.pool.QueryRow(r.Context(), `SELECT count(*),COALESCE(min(college_id::text),'') FROM external_college_access WHERE user_id=$1 AND access_kind='mentor' AND revoked_at IS NULL AND ($2='' OR college_id::text=$2)`, s.UserID, scope).Scan(&count, &selected)
		if err != nil {
			h.err.Handle(w, r, err)
			return s, false
		}
		if count != 1 {
			status, code, message := http.StatusForbidden, "mentor_scope_required", "an active college-scoped mentor invitation is required"
			if count > 1 && scope == "" {
				status, code, message = http.StatusBadRequest, "tenant_scope_required", "choose the college scope for this request"
			}
			h.fail(w, r, status, code, message, nil)
			return s, false
		}
		s.CollegeID = selected
	}
	if s.CollegeID == "" || !s.TermsAccepted || !s.PrivacyAccepted {
		h.fail(w, r, http.StatusForbidden, "campus_scope_required", "verified campus access and policy consent are required", nil)
		return s, false
	}
	return s, true
}

func (h *Handler) admin(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := h.session(w, r)
	if !ok {
		return s, false
	}
	if !s.CampusAdmin || !s.MFAEnabled || !s.MFAFresh {
		h.fail(w, r, http.StatusForbidden, "campus_admin_required", "a recently verified campus administrator session is required", nil)
		return s, false
	}
	return s, true
}

func (h *Handler) faculty(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := h.session(w, r)
	if !ok {
		return s, false
	}
	if s.CampusAdmin {
		return s, true
	}
	var active bool
	err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM faculty_profiles WHERE college_id=$1 AND user_id=$2 AND status='active')`, s.CollegeID, s.UserID).Scan(&active)
	if err != nil {
		h.err.Handle(w, r, err)
		return s, false
	}
	if !active {
		h.fail(w, r, http.StatusForbidden, "verified_faculty_required", "verified faculty access is required", nil)
		return s, false
	}
	return s, true
}

func (h *Handler) mentorshipSession(w http.ResponseWriter, r *http.Request, conversationID string) (identity.Session, bool) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return s, false
	}
	if !s.TermsAccepted || !s.PrivacyAccepted {
		h.fail(w, r, http.StatusForbidden, "policy_consent_required", "current policy consent is required", nil)
		return s, false
	}
	if s.AccountType != "external" {
		if s.CollegeID != "" {
			return s, true
		}
		h.fail(w, r, http.StatusForbidden, "campus_scope_required", "campus access is required", nil)
		return s, false
	}
	err := h.pool.QueryRow(r.Context(), `SELECT mc.college_id::text FROM mentorship_conversations mc JOIN mentorship_requests mr ON mr.id=mc.mentorship_request_id AND mr.college_id=mc.college_id JOIN mentor_profiles mp ON mp.id=mr.mentor_profile_id AND mp.college_id=mr.college_id WHERE mc.id=$1 AND mp.user_id=$2 AND EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.user_id=$2 AND ea.college_id=mc.college_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL)`, conversationID, s.UserID).Scan(&s.CollegeID)
	if err != nil {
		h.fail(w, r, http.StatusNotFound, "mentorship_conversation_not_found", "active mentorship conversation was not found", err)
		return s, false
	}
	return s, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	h.err.Handle(w, r, errormanager.New(status, code, message, cause))
}
