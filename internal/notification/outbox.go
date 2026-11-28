package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/campus"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/outbox"
)

type eventPayload struct {
	ProjectID      string `json:"project_id"`
	ApplicationID  string `json:"application_id"`
	UserID         string `json:"user_id"`
	ActorID        string `json:"actor_id"`
	ConversationID string `json:"conversation_id"`
}
type recipient struct{ userID, category, title, body, projectID, applicationID string }

func OutboxHandler(pool *pgxpool.Pool, mail Mail) outbox.Handler {
	return func(ctx context.Context, d outbox.Delivery) error {
		items, err := notificationRecipients(ctx, pool, d)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		for _, item := range items {
			var existing bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notifications WHERE source_event_id=$1 AND user_id=$2)`, d.EventID, item.userID).Scan(&existing); err != nil {
				return err
			}
			if existing {
				continue
			}
			var sequence int64
			if err = tx.QueryRow(ctx, `INSERT INTO notification_cursors(college_id,user_id,last_sequence) VALUES($1,$2,1) ON CONFLICT(college_id,user_id) DO UPDATE SET last_sequence=notification_cursors.last_sequence+1 RETURNING last_sequence`, d.TenantID, item.userID).Scan(&sequence); err != nil {
				return err
			}
			var notificationID string
			err = tx.QueryRow(ctx, `INSERT INTO notifications(college_id,user_id,source_event_id,sequence,category,event_type,title,body,project_id,application_id)
				SELECT $1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,NULLIF($10,'')::uuid WHERE EXISTS(SELECT 1 FROM users u WHERE u.id=$2 AND u.college_id=$1 AND u.status='active')
				ON CONFLICT(source_event_id,user_id) DO NOTHING RETURNING id::text`, d.TenantID, item.userID, d.EventID, sequence, item.category, d.EventType, item.title, item.body, item.projectID, item.applicationID).Scan(&notificationID)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if item.category == "applications" {
				_, err = tx.Exec(ctx, `INSERT INTO notification_preferences(college_id,user_id,category,email_enabled) VALUES($1,$2,'applications',true) ON CONFLICT DO NOTHING`, d.TenantID, item.userID)
			} else if item.category == "messages" {
				_, err = tx.Exec(ctx, `INSERT INTO notification_preferences(college_id,user_id,category,email_enabled) VALUES($1,$2,'messages',false) ON CONFLICT DO NOTHING`, d.TenantID, item.userID)
			} else {
				_, err = tx.Exec(ctx, `INSERT INTO notification_preferences(college_id,user_id,category,email_enabled) VALUES($1,$2,'project_activity',false) ON CONFLICT DO NOTHING`, d.TenantID, item.userID)
			}
			if err != nil {
				return err
			}
			if item.category == "messages" {
				continue
			}
			var enabled bool
			if err = tx.QueryRow(ctx, `SELECT email_enabled FROM notification_preferences WHERE college_id=$1 AND user_id=$2 AND category=$3`, d.TenantID, item.userID, item.category).Scan(&enabled); err != nil {
				return err
			}
			if !enabled || mail.AppOrigin == "" {
				continue
			}
			var eligible bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN colleges c ON c.id=u.college_id JOIN campus_verifications v ON v.college_id=c.id AND v.user_id=u.id AND v.revoked_at IS NULL WHERE u.id=$1 AND u.college_id=$2 AND u.status='active' AND c.status='active')`, item.userID, d.TenantID).Scan(&eligible)
			if err != nil {
				return err
			}
			if !eligible {
				continue
			}
			var deliveryID string
			if err = tx.QueryRow(ctx, `INSERT INTO notification_email_deliveries(notification_id,college_id,user_id) VALUES($1,$2,$3) RETURNING id::text`, notificationID, d.TenantID, item.userID).Scan(&deliveryID); err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]string{"delivery_id": deliveryID})
			if _, err = jobs.EnqueueTx(ctx, tx, d.TenantID, item.userID, "notification_email", "notification/"+deliveryID, payload, nil); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	}
}

func notificationRecipients(ctx context.Context, pool *pgxpool.Pool, d outbox.Delivery) ([]recipient, error) {
	p := eventPayload{}
	if len(d.Payload) > 0 && json.Unmarshal(d.Payload, &p) != nil {
		return nil, errors.New("notification event payload is invalid")
	}
	switch d.EventType {
	case "conversation.message.created":
		if p.ConversationID == "" || p.ActorID == "" {
			return nil, errors.New("conversation message notification is missing its context")
		}
		rows, err := pool.Query(ctx, `SELECT cp.user_id::text FROM conversations c
			JOIN conversation_participants cp ON cp.conversation_id=c.id AND cp.college_id=c.college_id
			JOIN users u ON u.id=cp.user_id AND u.college_id=c.college_id AND u.status='active'
			JOIN campus_verifications cv ON cv.college_id=u.college_id AND cv.user_id=u.id AND cv.revoked_at IS NULL
			WHERE c.id=$1 AND c.college_id=$2 AND c.state='open' AND cp.user_id<>$3
			AND EXISTS(SELECT 1 FROM consent_records cr WHERE cr.user_id=cp.user_id AND cr.purpose='terms' AND cr.revoked_at IS NULL)
			AND EXISTS(SELECT 1 FROM consent_records cr WHERE cr.user_id=cp.user_id AND cr.purpose='privacy' AND cr.revoked_at IS NULL)
			AND NOT EXISTS(SELECT 1 FROM user_blocks b WHERE b.college_id=c.college_id AND ((b.blocker_id=cp.user_id AND b.blocked_id=$3) OR (b.blocker_id=$3 AND b.blocked_id=cp.user_id)))
			AND CASE c.context_type
				WHEN 'project' THEN EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=c.project_id AND pm.college_id=c.college_id AND pm.user_id=cp.user_id)
				WHEN 'application' THEN EXISTS(SELECT 1 FROM applications a JOIN project_roles rr ON rr.id=a.project_role_id AND rr.college_id=a.college_id
					WHERE a.id=c.application_id AND a.college_id=c.college_id AND a.status='accepted'
					AND (a.applicant_user_id=cp.user_id OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=rr.project_id AND pm.college_id=rr.college_id AND pm.user_id=cp.user_id AND pm.role IN ('owner','maintainer'))))
				ELSE EXISTS(SELECT 1 FROM pbl_conversation_users v WHERE v.conversation_id=c.id AND v.college_id=c.college_id AND v.user_id=cp.user_id)
			END`, p.ConversationID, d.TenantID, p.ActorID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := []recipient{}
		for rows.Next() {
			var user string
			if err = rows.Scan(&user); err != nil {
				return nil, err
			}
			items = append(items, recipient{userID: user, category: "messages", title: "New message", body: "A participant sent a message in a conversation you can access.", projectID: p.ProjectID, applicationID: p.ApplicationID})
		}
		return items, rows.Err()
	case "application.created":
		var applicant, project string
		err := pool.QueryRow(ctx, `SELECT a.applicant_user_id::text,r.project_id::text FROM applications a JOIN project_roles r ON r.id=a.project_role_id AND r.college_id=a.college_id WHERE a.id=$1 AND a.college_id=$2`, d.AggregateID, d.TenantID).Scan(&applicant, &project)
		if err != nil {
			return nil, err
		}
		rows, err := pool.Query(ctx, `SELECT user_id::text FROM project_members WHERE project_id=$1 AND college_id=$2 AND role IN ('owner','maintainer') AND user_id<>$3`, project, d.TenantID, applicant)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []recipient{}
		for rows.Next() {
			var user string
			if err = rows.Scan(&user); err != nil {
				return nil, err
			}
			out = append(out, recipient{userID: user, category: "applications", title: "New project application", body: "A student applied to join a project you manage.", projectID: project, applicationID: d.AggregateID})
		}
		return out, rows.Err()
	case "application.accepted", "application.rejected":
		if p.UserID == "" || p.ProjectID == "" {
			return nil, errors.New("application decision is missing recipient or project")
		}
		title := "Application update"
		body := "Your project application has a new decision."
		return []recipient{{userID: p.UserID, category: "applications", title: title, body: body, projectID: p.ProjectID, applicationID: first(p.ApplicationID, d.AggregateID)}}, nil
	case "project.update.created", "project.discussion.created", "project.task.created", "project.task.updated", "project.milestone.created", "project.milestone.updated", "project.comment.created", "project.updated":
		if p.ProjectID == "" && d.EventType == "project.updated" {
			p.ProjectID = d.AggregateID
		}
		if p.ProjectID == "" {
			return nil, errors.New("project activity is missing project ID")
		}
		actor := p.ActorID
		if actor == "" {
			switch d.AggregateType {
			case "project_update":
				_ = pool.QueryRow(ctx, `SELECT author_id::text FROM project_updates WHERE id=$1`, d.AggregateID).Scan(&actor)
			case "project_discussion":
				_ = pool.QueryRow(ctx, `SELECT author_id::text FROM project_discussions WHERE id=$1`, d.AggregateID).Scan(&actor)
			case "project_task":
				_ = pool.QueryRow(ctx, `SELECT actor_id::text FROM project_task_events WHERE task_id=$1 AND version=$2`, d.AggregateID, d.Version).Scan(&actor)
			case "project_milestone":
				_ = pool.QueryRow(ctx, `SELECT created_by::text FROM project_milestones WHERE id=$1`, d.AggregateID).Scan(&actor)
			case "project_comment":
				_ = pool.QueryRow(ctx, `SELECT author_id::text FROM project_comments WHERE id=$1`, d.AggregateID).Scan(&actor)
			}
		}
		rows, err := pool.Query(ctx, `SELECT user_id::text FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id<>NULLIF($3,'')::uuid`, p.ProjectID, d.TenantID, actor)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []recipient{}
		title, body := activityText(d.EventType)
		for rows.Next() {
			var user string
			if err = rows.Scan(&user); err != nil {
				return nil, err
			}
			out = append(out, recipient{userID: user, category: "project_activity", title: title, body: body, projectID: p.ProjectID})
		}
		return out, rows.Err()
	case "project.update.edited", "project.discussion.updated", "project.reaction.set", "project.reaction.removed", "project.content.reported", "project.followed", "project.bookmarked", "project.unfollowed", "project.unbookmarked":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported notification outbox event: %s", d.EventType)
	}
}

func activityText(event string) (string, string) {
	switch event {
	case "project.update.created":
		return "New project update", "There is new activity in a project you can access."
	case "project.discussion.created":
		return "New project discussion", "A discussion was started in a project you can access."
	case "project.task.created":
		return "New project task", "A task was added to a project you can access."
	case "project.task.updated":
		return "Project task updated", "A task changed in a project you can access."
	case "project.milestone.created", "project.milestone.updated":
		return "Project milestone updated", "A milestone changed in a project you can access."
	case "project.comment.created":
		return "New project comment", "There is a new comment in a project you can access."
	default:
		return "Project updated", "A project you can access was updated."
	}
}
func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func EmailJobHandler(pool *pgxpool.Pool, mail Mail, smtp campus.MailConfig) jobs.Handler {
	return func(ctx context.Context, claim jobs.Claim) (string, error) {
		var payload struct {
			DeliveryID string `json:"delivery_id"`
		}
		if json.Unmarshal(claim.Payload, &payload) != nil || payload.DeliveryID == "" {
			return "", errors.New("notification email job is invalid")
		}
		var user, to, category, projectID, applicationID, status string
		err := pool.QueryRow(ctx, `SELECT d.user_id::text,u.email_normalized,n.category,COALESCE(n.project_id::text,''),COALESCE(n.application_id::text,''),d.status FROM notification_email_deliveries d JOIN users u ON u.id=d.user_id JOIN notifications n ON n.id=d.notification_id WHERE d.id=$1 AND d.college_id=$2`, payload.DeliveryID, claim.TenantID).Scan(&user, &to, &category, &projectID, &applicationID, &status)
		if err != nil {
			return "", err
		}
		if status != "queued" && status != "failed" {
			return "", nil
		}
		var authorized bool
		err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN colleges c ON c.id=u.college_id JOIN campus_verifications v ON v.college_id=c.id AND v.user_id=u.id AND v.revoked_at IS NULL WHERE u.id=$1 AND u.college_id=$2 AND u.status='active' AND c.status='active' AND EXISTS(SELECT 1 FROM notification_preferences p WHERE p.college_id=$2 AND p.user_id=$1 AND p.category=$3 AND p.email_enabled) AND ((NULLIF($4,'') IS NULL OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=NULLIF($4,'')::uuid AND m.user_id=$1)) OR (NULLIF($5,'') IS NOT NULL AND EXISTS(SELECT 1 FROM applications a WHERE a.id=NULLIF($5,'')::uuid AND a.applicant_user_id=$1))) AND (NULLIF($5,'') IS NULL OR EXISTS(SELECT 1 FROM applications a WHERE a.id=NULLIF($5,'')::uuid AND a.applicant_user_id=$1) OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=NULLIF($4,'')::uuid AND m.user_id=$1)))`, user, claim.TenantID, category, projectID, applicationID).Scan(&authorized)
		if err != nil {
			return "", err
		}
		if !authorized {
			_, err = pool.Exec(ctx, `UPDATE notification_email_deliveries SET status='suppressed',status_at=clock_timestamp() WHERE id=$1`, payload.DeliveryID)
			if err != nil {
				return "", err
			}
			_, err = pool.Exec(ctx, `UPDATE notification_email_attempts SET result='suppressed' WHERE delivery_id=$1 AND attempt=$2`, payload.DeliveryID, claim.Attempt)
			return "", err
		}
		var suppressed bool
		if mail.SuppressionKey != "" {
			err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM email_suppressions WHERE recipient_key=$1)`, campus.RecipientKey(mail.SuppressionKey, to)).Scan(&suppressed)
			if err != nil {
				return "", err
			}
		}
		if suppressed {
			_, err = pool.Exec(ctx, `UPDATE notification_email_deliveries SET status='suppressed',status_at=clock_timestamp() WHERE id=$1`, payload.DeliveryID)
			if err != nil {
				return "", err
			}
			_, err = pool.Exec(ctx, `INSERT INTO notification_email_attempts(delivery_id,attempt,provider,result) VALUES($1,$2,$3,'suppressed') ON CONFLICT(delivery_id,attempt) DO UPDATE SET result='suppressed'`, payload.DeliveryID, claim.Attempt, provider(mail.Sender))
			return "", err
		}
		if _, err = pool.Exec(ctx, `INSERT INTO notification_email_attempts(delivery_id,attempt,provider,result) VALUES($1,$2,$3,'started') ON CONFLICT DO NOTHING`, payload.DeliveryID, claim.Attempt, provider(mail.Sender)); err != nil {
			return "", err
		}
		subject := "PeerGit project activity"
		textBody := "There is a new notification in your PeerGit inbox. Open PeerGit to view it."
		link := strings.TrimRight(mail.AppOrigin, "/") + "/inbox"
		htmlBody := `<p>There is a new notification in your PeerGit inbox.</p><p><a href="` + htmlAttr(link) + `">Open PeerGit</a></p>`
		var messageID string
		if mail.Sender != nil {
			from := mail.From
			if from == "" {
				from = "PeerGit <notifications@mail.peergit.com>"
			}
			messageID, err = mail.Sender.SendNotification(ctx, payload.DeliveryID, from, to, subject, textBody, htmlBody)
		} else {
			err = campus.SendSMTPNotification(ctx, smtp, to, subject, textBody, htmlBody)
		}
		if err != nil {
			_, _ = pool.Exec(ctx, `UPDATE notification_email_deliveries SET attempts=GREATEST(attempts,$2),status='failed',status_at=clock_timestamp(),last_error_code='provider_error' WHERE id=$1`, payload.DeliveryID, claim.Attempt)
			_, _ = pool.Exec(ctx, `UPDATE notification_email_attempts SET result='failed',error_code='provider_error' WHERE delivery_id=$1 AND attempt=$2`, payload.DeliveryID, claim.Attempt)
			return "", err
		}
		providerName := provider(mail.Sender)
		if _, err = pool.Exec(ctx, `UPDATE notification_email_deliveries SET attempts=GREATEST(attempts,$4),provider=$2,provider_message_id=NULLIF($3,''),status='accepted',status_at=clock_timestamp(),last_error_code=NULL WHERE id=$1`, payload.DeliveryID, providerName, messageID, claim.Attempt); err != nil {
			return "", err
		}
		_, err = pool.Exec(ctx, `UPDATE notification_email_attempts SET result='accepted',provider_message_id=NULLIF($3,'') WHERE delivery_id=$1 AND attempt=$2`, payload.DeliveryID, claim.Attempt, messageID)
		return "", err
	}
}

func provider(sender MailSender) string {
	if sender != nil {
		return "resend"
	}
	return "smtp"
}
func htmlAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "\"", "&quot;", "<", "&lt;", ">", "&gt;").Replace(s)
}
