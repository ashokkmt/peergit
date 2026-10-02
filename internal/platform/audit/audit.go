package audit

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type Entry struct {
	TenantID, ActorID, Action, ResourceType, ResourceID, RequestID string
	Details                                                        json.RawMessage
}

func Append(ctx context.Context, tx pgx.Tx, entry Entry) error {
	if entry.TenantID == "" || entry.ActorID == "" || entry.Action == "" || entry.ResourceType == "" {
		return errors.New("tenant, actor, action, and resource type are required")
	}
	if len(entry.Details) == 0 {
		entry.Details = json.RawMessage(`{}`)
	}
	if !json.Valid(entry.Details) {
		return errors.New("audit details must be valid JSON")
	}
	_, err := tx.Exec(ctx, `INSERT INTO audit_log(tenant_id,actor_id,action,resource_type,resource_id,request_id,details)
	 VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,''),$7)`, entry.TenantID, entry.ActorID, entry.Action, entry.ResourceType, entry.ResourceID, entry.RequestID, entry.Details)
	return err
}
