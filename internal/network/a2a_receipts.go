package network

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/jackc/pgx/v5"
)

type coordinationCommand struct {
	Version   string                  `json:"version"`
	Operation string                  `json:"operation"`
	Arguments networkCommandArguments `json:"arguments"`
}

type a2aReceipt struct {
	id, principal, agent, context, message, operation, key, requestHash, resultHash, resource string
	sequence                                                                                  int64
	status                                                                                    int
	created                                                                                   time.Time
}

// These are typed command execution receipts, not serialized private results.
func (n *networkA2A) SendMessage(ctx context.Context, in *a2a.SendMessageRequest) (a2a.SendMessageResult, error) {
	original, err := originalA2ARequest(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Message == nil || in.Message.Role != a2a.MessageRoleUser || len(in.Message.ID) < 1 || len(in.Message.ID) > 128 || len(in.Message.Parts) != 1 || in.Message.Parts[0] == nil || in.Tenant != "" || in.Message.TaskID != "" || len(in.Message.ReferenceTasks) > 0 {
		return nil, a2a.ErrInvalidParams
	}
	if in.Config != nil && in.Config.PushConfig != nil {
		return nil, a2a.ErrPushNotificationNotSupported
	}
	if err := validateA2AOutputMode(in); err != nil {
		return nil, err
	}
	switch content := in.Message.Parts[0].Content.(type) {
	case a2a.Text:
		if len(content) > 16000 {
			return nil, a2a.ErrInvalidParams
		}
		return a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("Text is untrusted, nonexecuting content. Use the explicitly activated Undergrid coordination extension and one typed data command.")), nil
	case a2a.Data:
		if in.Message.Parts[0].MediaType != "" && in.Message.Parts[0].MediaType != "application/json" {
			return nil, a2a.ErrUnsupportedContentType
		}
		return n.executeCommand(ctx, original, in, content.Value)
	default:
		return nil, a2a.ErrUnsupportedContentType
	}
}

// A response is usable only if its concrete part media type is acceptable.
// This transport check is shared by HTTP pre-dispatch and the SDK handler.
func validateA2AOutputMode(in *a2a.SendMessageRequest) error {
	if in == nil || in.Message == nil || len(in.Message.Parts) != 1 || in.Message.Parts[0] == nil || in.Config == nil || len(in.Config.AcceptedOutputModes) == 0 {
		return nil
	}
	outputMode := "application/json"
	if _, ok := in.Message.Parts[0].Content.(a2a.Text); ok {
		outputMode = "text/plain"
	}
	if !slices.Contains(in.Config.AcceptedOutputModes, outputMode) {
		return a2a.ErrUnsupportedContentType
	}
	return nil
}

func (n *networkA2A) executeCommand(ctx context.Context, original *http.Request, in *a2a.SendMessageRequest, value any) (a2a.SendMessageResult, error) {
	raw := canonical(value)
	var command coordinationCommand
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&command) != nil || command.Version != "1" {
		return nil, a2a.ErrInvalidParams
	}
	op, ok := lookupNetworkOperation(command.Operation)
	if !ok {
		return nil, a2a.ErrInvalidParams
	}
	resolved := n.schemas[op.name]
	var args any
	if json.Unmarshal(canonical(command.Arguments), &args) != nil || resolved == nil || resolved.Validate(args) != nil {
		return nil, a2a.ErrInvalidParams
	}
	// Waiting belongs outside domain transactions. A2A's non-streaming inbox
	// operation exposes an immediate snapshot, not a fictitious long poll.
	if wait, ok := command.Arguments.Query["wait_seconds"]; ok && fmt.Sprint(wait) != "0" {
		return nil, a2a.ErrUnsupportedOperation
	}
	request, err := buildNetworkCommandRequest(ctx, original.Header.Get("Authorization"), op, command.Arguments)
	if err != nil {
		return nil, a2a.ErrInvalidParams
	}
	db, tx, err := n.service.beginNetworkTransaction(ctx)
	if err != nil {
		return nil, a2a.ErrInternalError
	}
	defer db.Close(ctx)
	defer tx.Rollback(ctx)
	a, err := authenticateAgentRequest(request, tx)
	if err != nil {
		return nil, a2aDomainProtocolError(err)
	}
	key := command.Arguments.IdempotencyKey
	if key == "" {
		key = in.Message.ID
	}
	requestHash := digest(canonical(in))
	old, found, err := loadA2AReplay(ctx, tx, a, in.Message.ID, op.name, key)
	if err != nil {
		var conflict problem
		if errors.As(err, &conflict) && conflict.Status == 409 {
			return a2aDomainRejection(err), nil
		}
		return nil, a2a.ErrInternalError
	}
	if found && (old.message != in.Message.ID || old.operation != op.name || old.key != key || old.requestHash != requestHash) {
		return a2aDomainRejection(fail(409, "idempotency_conflict")), nil
	}
	if !found && in.Message.ContextID != "" {
		if !validID(in.Message.ContextID) {
			return nil, a2a.ErrInvalidParams
		}
		var owned bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM a2a_command_receipts WHERE agent_id=$1 AND context_id=$2)", a.AgentID, in.Message.ContextID).Scan(&owned); err != nil {
			return nil, a2a.ErrInternalError
		}
		if !owned {
			return nil, a2a.ErrTaskNotFound
		}
	}
	// Replays re-enter the same domain authorization/idempotency path before
	// disclosing a response. No transport cache can resurrect revoked authority.
	status, result, err := n.service.agentCommand(request, tx, a)
	if err != nil {
		var p problem
		if errors.As(err, &p) && p.Status >= 400 && p.Status < 500 && p.Status != 401 && p.Status != 403 {
			return a2aDomainRejection(err), nil
		}
		return nil, a2aDomainProtocolError(err)
	}
	receipt := old
	if !found {
		contextID := in.Message.ContextID
		if contextID == "" {
			contextID = NewID()
		}
		receipt = a2aReceipt{id: NewID(), principal: a.PrincipalID, agent: a.AgentID, context: contextID, message: in.Message.ID, operation: op.name, key: key, requestHash: requestHash, resultHash: digest(canonical(result)), status: status, resource: a2aResourcePath(op, request, result)}
		err = tx.QueryRow(ctx, `INSERT INTO a2a_command_receipts(id,principal_id,agent_id,context_id,message_id,operation,command_key,request_hash,result_hash,command_status,resource_path,sequence)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,COALESCE(max(sequence),0)+1 FROM a2a_command_receipts WHERE agent_id=$3 RETURNING sequence,created_at`, receipt.id, receipt.principal, receipt.agent, receipt.context, receipt.message, receipt.operation, receipt.key, receipt.requestHash, receipt.resultHash, receipt.status, receipt.resource).Scan(&receipt.sequence, &receipt.created)
		if err != nil {
			return nil, a2a.ErrInternalError
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, a2a.ErrInternalError
	}
	// The authorized live response is not persisted in the receipt. Artifact and
	// room bytes cannot later be recovered from transport history.
	task := receipt.task()
	task.Artifacts = []*a2a.Artifact{{ID: a2a.ArtifactID(receipt.id + "-command-response"), Name: "Authorized command response", Parts: a2a.ContentParts{a2a.NewDataPart(map[string]any{"version": "1", "kind": "command_response", "operation": op.name, "status": status, "result": result})}}}
	return task, nil
}

func a2aDomainRejection(err error) *a2a.Message {
	var p problem
	if !errors.As(err, &p) {
		p = problem{500, "internal_error"}
	}
	return a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewDataPart(map[string]any{"version": "1", "kind": "command_rejected", "committed": false, "error": map[string]any{"code": p.Code, "status": p.Status, "retryable": p.Status == 429}}))
}
func a2aDomainProtocolError(err error) error {
	var p problem
	if errors.As(err, &p) {
		if p.Status == 401 {
			return a2a.ErrUnauthenticated
		}
		if p.Status == 403 {
			return a2a.NewError(a2a.ErrUnauthorized, "Current network authority does not permit this operation")
		}
		if p.Status == 404 {
			return a2a.ErrTaskNotFound
		}
	}
	return a2a.ErrInternalError
}

const a2aReceiptColumns = `id,principal_id,agent_id,context_id,message_id,operation,command_key,request_hash,result_hash,command_status,resource_path,sequence,created_at`

func scanA2AReceipt(row pgx.Row) (a2aReceipt, error) {
	var r a2aReceipt
	err := row.Scan(&r.id, &r.principal, &r.agent, &r.context, &r.message, &r.operation, &r.key, &r.requestHash, &r.resultHash, &r.status, &r.resource, &r.sequence, &r.created)
	return r, err
}
func loadA2AReplay(ctx context.Context, tx pgx.Tx, a actor, message, operation, key string) (a2aReceipt, bool, error) {
	rows, err := tx.Query(ctx, "SELECT "+a2aReceiptColumns+" FROM a2a_command_receipts WHERE agent_id=$1 AND (message_id=$2 OR (operation=$3 AND command_key=$4)) ORDER BY id", a.AgentID, message, operation, key)
	if err != nil {
		return a2aReceipt{}, false, err
	}
	defer rows.Close()
	var receipt a2aReceipt
	count := 0
	for rows.Next() {
		receipt, err = scanA2AReceipt(rows)
		if err != nil {
			return receipt, false, err
		}
		count++
	}
	if count > 1 {
		return receipt, false, fail(409, "idempotency_conflict")
	}
	return receipt, count == 1, rows.Err()
}
func (r a2aReceipt) task() *a2a.Task {
	return &a2a.Task{ID: a2a.TaskID(r.id), ContextID: r.context, Status: a2a.TaskStatus{State: a2a.TaskStateCompleted, Timestamp: &r.created}, Metadata: map[string]any{coordinationExtensionURI: map[string]any{"version": "1", "kind": "command_receipt", "operation": r.operation, "command_status": r.status, "result_sha256": r.resultHash, "semantics": "command_committed_not_assignment_completion"}}}
}

func a2aResourcePath(op networkOperation, r *http.Request, result any) string {
	path := r.URL.Path
	value, _ := result.(map[string]any)
	id, _ := value["id"].(string)
	if op.method == "GET" {
		return path
	}
	if op.name == "intention_create" && validID(id) {
		return "/v1/intents/" + id
	}
	if op.name == "swarm_create" && validID(id) {
		return "/v1/swarms/" + id + "/context"
	}
	if op.name == "plan_propose" && validID(id) {
		return "/v1/plans/" + id
	}
	if op.name == "report_create" && validID(id) {
		return "/v1/reports/" + id
	}
	if op.name == "artifact_upload_request" && validID(id) {
		return "/v1/artifacts/" + id
	}
	if strings.HasPrefix(path, "/v1/intents/") {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && validID(parts[3]) {
		switch parts[2] {
		case "swarms":
			return "/v1/swarms/" + parts[3] + "/context"
		case "plans", "tasks", "artifacts", "artifact-input-grants":
			return "/v1/" + parts[2] + "/" + parts[3]
		}
	}
	return ""
}

type a2aPageCursor struct {
	Agent, Environment, Context, Status string
	After                               int64
	Since                               string
	Issued                              int64
}

func (n *networkA2A) pageCursor(a actor, in *a2a.ListTasksRequest, after int64) string {
	since := ""
	if in.StatusTimestampAfter != nil {
		since = in.StatusTimestampAfter.UTC().Format(time.RFC3339Nano)
	}
	raw := canonical(a2aPageCursor{a.AgentID, n.service.environment, in.ContextID, string(in.Status), after, since, time.Now().Unix()})
	mac := hmac.New(sha256.New, n.service.cursorKey)
	mac.Write([]byte("undergrid-a2a-page-v1\x00"))
	mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (n *networkA2A) decodePageCursor(a actor, in *a2a.ListTasksRequest) (int64, error) {
	if in.PageToken == "" {
		return 0, nil
	}
	if len(in.PageToken) > 2048 {
		return 0, a2a.ErrInvalidParams
	}
	parts := strings.Split(in.PageToken, ".")
	if len(parts) != 2 {
		return 0, a2a.ErrInvalidParams
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	sig, e2 := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil || e2 != nil {
		return 0, a2a.ErrInvalidParams
	}
	mac := hmac.New(sha256.New, n.service.cursorKey)
	mac.Write([]byte("undergrid-a2a-page-v1\x00"))
	mac.Write(raw)
	var cursor a2aPageCursor
	since := ""
	if in.StatusTimestampAfter != nil {
		since = in.StatusTimestampAfter.UTC().Format(time.RFC3339Nano)
	}
	if !hmac.Equal(sig, mac.Sum(nil)) || json.Unmarshal(raw, &cursor) != nil || cursor.Agent != a.AgentID || cursor.Environment != n.service.environment || cursor.Context != in.ContextID || cursor.Status != string(in.Status) || cursor.Since != since || cursor.After < 0 || cursor.Issued > time.Now().Add(time.Minute).Unix() || cursor.Issued < time.Now().Add(-24*time.Hour).Unix() {
		return 0, a2a.ErrInvalidParams
	}
	return cursor.After, nil
}

func (n *networkA2A) GetTask(ctx context.Context, in *a2a.GetTaskRequest) (*a2a.Task, error) {
	if in == nil || in.Tenant != "" || !validID(string(in.ID)) || (in.HistoryLength != nil && *in.HistoryLength < 0) {
		return nil, a2a.ErrTaskNotFound
	}
	original, err := originalA2ARequest(ctx)
	if err != nil {
		return nil, err
	}
	request := original.Clone(ctx)
	request.Method = "GET"
	request.URL = &url.URL{Path: "/v1/me"}
	db, tx, err := n.service.beginNetworkTransaction(ctx)
	if err != nil {
		return nil, a2a.ErrInternalError
	}
	defer db.Close(ctx)
	defer tx.Rollback(ctx)
	a, err := authenticateAgentRequest(request, tx)
	if err != nil {
		return nil, a2aDomainProtocolError(err)
	}
	receipt, err := scanA2AReceipt(tx.QueryRow(ctx, "SELECT "+a2aReceiptColumns+" FROM a2a_command_receipts WHERE id=$1 AND agent_id=$2", string(in.ID), a.AgentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, a2a.ErrTaskNotFound
	}
	if err != nil {
		return nil, a2a.ErrInternalError
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, a2a.ErrInternalError
	}
	return n.recoverTask(ctx, original, receipt, true), nil
}

// Receipt recovery reads live resources through the existing authorized domain
// read path. A read denial suppresses the resource reference and data entirely;
// no private response body can be recovered from the receipt's minimal storage.
// These are independent read snapshots, not a second phase of a mutation.
func (n *networkA2A) recoverTask(ctx context.Context, original *http.Request, receipt a2aReceipt, includeResource bool) *a2a.Task {
	task := receipt.task()
	if !includeResource || receipt.resource == "" {
		return task
	}
	request := original.Clone(ctx)
	request.Method = "GET"
	request.URL = &url.URL{Path: receipt.resource}
	request.Body = http.NoBody
	_, current, err := n.service.execute(request)
	if err != nil {
		return task
	}
	task.Artifacts = []*a2a.Artifact{{ID: a2a.ArtifactID(receipt.id + "-current-resource"), Name: "Current authorized resource snapshot", Parts: a2a.ContentParts{a2a.NewDataPart(map[string]any{"version": "1", "kind": "current_resource_snapshot", "resource_path": receipt.resource, "resource": current})}}}
	return task
}

func (n *networkA2A) ListTasks(ctx context.Context, in *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	if in == nil || in.Tenant != "" || in.PageSize < 0 || in.PageSize > 100 || (in.ContextID != "" && !validID(in.ContextID)) || (in.HistoryLength != nil && *in.HistoryLength < 0) {
		return nil, a2a.ErrInvalidParams
	}
	if in.Status != "" && !slices.Contains([]a2a.TaskState{a2a.TaskStateCompleted, a2a.TaskStateCanceled, a2a.TaskStateFailed, a2a.TaskStateRejected, a2a.TaskStateSubmitted, a2a.TaskStateWorking, a2a.TaskStateInputRequired, a2a.TaskStateAuthRequired}, in.Status) {
		return nil, a2a.ErrInvalidParams
	}
	original, err := originalA2ARequest(ctx)
	if err != nil {
		return nil, err
	}
	request := original.Clone(ctx)
	request.Method = "GET"
	request.URL = &url.URL{Path: "/v1/me"}
	db, tx, err := n.service.beginNetworkTransaction(ctx)
	if err != nil {
		return nil, a2a.ErrInternalError
	}
	defer db.Close(ctx)
	defer tx.Rollback(ctx)
	a, err := authenticateAgentRequest(request, tx)
	if err != nil {
		return nil, a2aDomainProtocolError(err)
	}
	after, err := n.decodePageCursor(a, in)
	if err != nil {
		return nil, err
	}
	limit := in.PageSize
	if limit == 0 {
		limit = 50
	}
	if in.Status != "" && in.Status != a2a.TaskStateCompleted {
		return &a2a.ListTasksResponse{Tasks: []*a2a.Task{}, PageSize: limit}, nil
	}
	var contextFilter any
	if in.ContextID != "" {
		contextFilter = in.ContextID
	}
	var since any
	if in.StatusTimestampAfter != nil {
		since = in.StatusTimestampAfter.UTC()
	}
	var total int
	filter := ` FROM a2a_command_receipts WHERE agent_id=$1 AND ($2::uuid IS NULL OR context_id=$2) AND ($3::timestamptz IS NULL OR created_at>$3)`
	if err = tx.QueryRow(ctx, "SELECT count(*)"+filter, a.AgentID, contextFilter, since).Scan(&total); err != nil {
		return nil, a2a.ErrInternalError
	}
	rows, err := tx.Query(ctx, "SELECT "+a2aReceiptColumns+filter+" AND sequence>$4 ORDER BY sequence LIMIT $5", a.AgentID, contextFilter, since, after, limit+1)
	if err != nil {
		return nil, a2a.ErrInternalError
	}
	receipts := []a2aReceipt{}
	for rows.Next() {
		r, e := scanA2AReceipt(rows)
		if e != nil {
			rows.Close()
			return nil, a2a.ErrInternalError
		}
		receipts = append(receipts, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, a2a.ErrInternalError
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, a2a.ErrInternalError
	}
	out := &a2a.ListTasksResponse{Tasks: []*a2a.Task{}, TotalSize: total, PageSize: limit}
	if len(receipts) > limit {
		receipts = receipts[:limit]
		out.NextPageToken = n.pageCursor(a, in, receipts[len(receipts)-1].sequence)
	}
	for _, receipt := range receipts {
		out.Tasks = append(out.Tasks, n.recoverTask(ctx, original, receipt, in.IncludeArtifacts))
	}
	return out, nil
}
func (n *networkA2A) CancelTask(ctx context.Context, in *a2a.CancelTaskRequest) (*a2a.Task, error) {
	if in == nil || in.Tenant != "" {
		return nil, a2a.ErrInvalidParams
	}
	if _, err := n.GetTask(ctx, &a2a.GetTaskRequest{ID: in.ID}); err != nil {
		return nil, err
	}
	return nil, a2a.ErrTaskNotCancelable
}
