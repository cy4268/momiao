package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	opsRemoteClaimLease   = time.Minute
	opsRemoteStepTimeout  = 35 * time.Second
	opsRemoteIdleInterval = 2 * time.Second
)

type opsRemoteClaim struct {
	operation OpsOperation
	command   json.RawMessage
	token     string
	attempt   int64
	handler   OpsDurableRemoteHandler
}

// RunRemoteDispatchStep claims at most one durable command. Both remote calls
// happen after the claim transaction commits, and every attempt queries the
// same operation id before deciding whether first execution is still needed.
func (s *OpsService) RunRemoteDispatchStep(ctx context.Context) (bool, error) {
	claim, found, err := s.claimRemoteDispatch(ctx)
	if err != nil || !found {
		return found, err
	}
	receipt, exists, err := claim.handler.QueryOperation(ctx, claim.operation.OperationType, claim.operation.OperationID)
	if err != nil {
		code := opsRemoteErrorCode(err, "OPS_REMOTE_QUERY_RETRY")
		var fault *OpsRemoteFault
		if errors.As(err, &fault) && fault != nil && fault.NeedsReview {
			finishErr := s.reviewRemoteDispatch(ctx, claim, code, nil)
			if finishErr != nil {
				return true, finishErr
			}
			return true, err
		}
		finishErr := s.retryRemoteDispatch(ctx, claim, code, "REMOTE_QUERY_RETRY")
		if finishErr != nil {
			return true, finishErr
		}
		return true, err
	}
	if !exists {
		receipt, err = claim.handler.ExecuteOperation(ctx, claim.operation.OperationType, append(json.RawMessage{}, claim.command...))
		if err != nil {
			return true, s.finishRemoteExecutionError(ctx, claim, err)
		}
	}
	canonicalReceipt, err := canonicalOpsObject(receipt, 32768)
	if err != nil {
		finishErr := s.reviewRemoteDispatch(ctx, claim, "OPS_REMOTE_RECEIPT_INVALID", nil)
		if finishErr != nil {
			return true, finishErr
		}
		return true, err
	}
	completion, err := claim.handler.AcceptRemoteReceipt(claim.operation, append(json.RawMessage{}, claim.command...), canonicalReceipt)
	if err != nil {
		finishErr := s.reviewRemoteDispatch(ctx, claim, "OPS_REMOTE_RECEIPT_INVALID", canonicalReceipt)
		if finishErr != nil {
			return true, finishErr
		}
		return true, err
	}
	switch completion.State {
	case "SUCCEEDED":
		if completion.FailureCode != "" {
			finishErr := s.reviewRemoteDispatch(ctx, claim, "OPS_REMOTE_RECEIPT_INVALID", canonicalReceipt)
			if finishErr != nil {
				return true, finishErr
			}
			return true, ErrOpsInvalid
		}
		audit, auditErr := normalizeOpsAudit(completion.Execution)
		if auditErr != nil {
			finishErr := s.reviewRemoteDispatch(ctx, claim, "OPS_REMOTE_AUDIT_REJECTED", canonicalReceipt)
			if finishErr != nil {
				return true, finishErr
			}
			return true, auditErr
		}
		audit, auditErr = bindOpsRemoteReceipt(audit, canonicalReceipt, completion.State)
		if auditErr != nil {
			finishErr := s.reviewRemoteDispatch(ctx, claim, "OPS_REMOTE_AUDIT_REJECTED", canonicalReceipt)
			if finishErr != nil {
				return true, finishErr
			}
			return true, auditErr
		}
		return true, s.succeedRemoteDispatch(ctx, claim, canonicalReceipt, completion.Execution, audit)
	case "NEEDS_REVIEW":
		code := opsRemoteCode(completion.FailureCode, "OPS_REMOTE_NEEDS_REVIEW")
		audit, auditErr := normalizeOpsAudit(completion.Execution)
		if auditErr == nil {
			audit, auditErr = bindOpsRemoteReceipt(audit, canonicalReceipt, completion.State)
		}
		if auditErr != nil {
			finishErr := s.reviewRemoteDispatch(ctx, claim, "OPS_REMOTE_AUDIT_REJECTED", canonicalReceipt)
			if finishErr != nil {
				return true, finishErr
			}
			return true, auditErr
		}
		return true, s.acceptRemoteReview(ctx, claim, code, canonicalReceipt, completion.Execution, audit)
	default:
		finishErr := s.reviewRemoteDispatch(ctx, claim, "OPS_REMOTE_RECEIPT_INVALID", canonicalReceipt)
		if finishErr != nil {
			return true, finishErr
		}
		return true, ErrOpsInvalid
	}
}

func (s *OpsService) claimRemoteDispatch(ctx context.Context) (opsRemoteClaim, bool, error) {
	if s == nil || s.store == nil || s.store.pool == nil || s.registry == nil {
		return opsRemoteClaim{}, false, ErrOpsUnavailable
	}
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return opsRemoteClaim{}, false, err
	}
	defer rollback(tx)
	var operationID, operationType string
	var command, commandHash []byte
	var attempts int64
	err = tx.QueryRow(ctx, `SELECT operation_id::text,operation_type,command_payload,command_hash,attempt_count
	 FROM ops.remote_dispatches
	 WHERE state IN('PENDING','RETRY') AND next_attempt_at<=clock_timestamp()
	    OR state='DISPATCHING' AND lease_expires_at<=clock_timestamp()
	 ORDER BY CASE WHEN state='DISPATCHING' THEN lease_expires_at ELSE next_attempt_at END,operation_id
	 LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&operationID, &operationType, &command, &commandHash, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return opsRemoteClaim{}, false, nil
	}
	if err != nil {
		return opsRemoteClaim{}, false, err
	}
	entry, known := s.registry.lookup(operationType)
	handler, remote := entry.handler.(OpsDurableRemoteHandler)
	if !known || !remote || entry.descriptor.ExecutionMode != OpsDurableRemote {
		return opsRemoteClaim{}, false, ErrOpsUnavailable
	}
	operation, err := loadOpsOperation(ctx, tx, operationID, true)
	if err != nil {
		return opsRemoteClaim{}, false, err
	}
	if operation.Environment != s.environment {
		return opsRemoteClaim{}, false, ErrOpsEnvironment
	}
	if operation.OperationType != operationType ||
		operation.State != "EXECUTING" && operation.State != "RECOVERING" {
		return opsRemoteClaim{}, false, ErrOpsConflict
	}
	canonicalCommand, err := canonicalOpsObject(command, 32768)
	if err != nil {
		return opsRemoteClaim{}, false, err
	}
	digest := sha256.Sum256(append([]byte("CHALDEA-OPS-REMOTE-COMMAND-V1\x00"), canonicalCommand...))
	if len(commandHash) != sha256.Size || !bytes.Equal(commandHash, digest[:]) {
		return opsRemoteClaim{}, false, ErrOpsConflict
	}
	token, err := uuidV7()
	if err != nil {
		return opsRemoteClaim{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE ops.remote_dispatches SET state='DISPATCHING',
	 attempt_count=attempt_count+1,next_attempt_at=NULL,claim_token=$2,
	 lease_expires_at=clock_timestamp()+$3::interval,failure_code=NULL,updated_at=clock_timestamp()
	 WHERE operation_id=$1`, operationID, token, opsRemoteClaimLease.String()); err != nil {
		return opsRemoteClaim{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return opsRemoteClaim{}, false, err
	}
	return opsRemoteClaim{operation: operation, command: canonicalCommand, token: token,
		attempt: attempts + 1, handler: handler}, true, nil
}

func (s *OpsService) finishRemoteExecutionError(ctx context.Context, claim opsRemoteClaim, remoteErr error) error {
	var fault *OpsRemoteFault
	if !errors.As(remoteErr, &fault) || fault == nil {
		code := opsRemoteErrorCode(remoteErr, "OPS_REMOTE_EXECUTE_RETRY")
		if err := s.retryRemoteDispatch(ctx, claim, code, "REMOTE_EXECUTE_RETRY"); err != nil {
			return err
		}
		return remoteErr
	}
	code := opsRemoteCode(fault.Code, "OPS_REMOTE_FAILURE")
	if fault.NeedsReview {
		return s.reviewRemoteDispatch(ctx, claim, code, nil)
	}
	if fault.Retryable {
		if err := s.retryRemoteDispatch(ctx, claim, code, "REMOTE_EXECUTE_RETRY"); err != nil {
			return err
		}
		return remoteErr
	}
	return s.failRemoteDispatch(ctx, claim, code)
}

func opsRemoteCode(code, fallback string) string {
	if opsOperationTypePattern.MatchString(code) {
		return code
	}
	return fallback
}

func opsRemoteErrorCode(err error, fallback string) string {
	var fault *OpsRemoteFault
	if errors.As(err, &fault) && fault != nil {
		return opsRemoteCode(fault.Code, fallback)
	}
	return fallback
}

func opsRemoteRetryDelay(attempt int64) time.Duration {
	switch {
	case attempt <= 1:
		return 5 * time.Second
	case attempt == 2:
		return 10 * time.Second
	case attempt == 3:
		return 20 * time.Second
	case attempt == 4:
		return 40 * time.Second
	default:
		return time.Minute
	}
}

func bindOpsRemoteReceipt(audit normalizedOpsAudit, receipt json.RawMessage, state string) (normalizedOpsAudit, error) {
	var result map[string]any
	if err := json.Unmarshal(audit.Result, &result); err != nil || result == nil {
		return normalizedOpsAudit{}, ErrOpsAuditRejected
	}
	digest := sha256.Sum256(append([]byte("CHALDEA-OPS-REMOTE-RECEIPT-V1\x00"), receipt...))
	result["remote_receipt_accepted"] = true
	result["remote_receipt_hash"] = hex.EncodeToString(digest[:])
	result["remote_receipt_state"] = state
	raw, err := json.Marshal(result)
	if err != nil {
		return normalizedOpsAudit{}, ErrOpsAuditRejected
	}
	audit.Result, err = canonicalOpsAuditObject(raw, 8192)
	if err != nil {
		return normalizedOpsAudit{}, err
	}
	return audit, nil
}

func opsRemoteFinalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}

func (s *OpsService) withRemoteClaim(ctx context.Context, claim opsRemoteClaim,
	apply func(context.Context, pgx.Tx, OpsOperation, time.Time) error) error {
	finishCtx, cancel := opsRemoteFinalizeContext(ctx)
	defer cancel()
	tx, err := s.store.pool.Begin(finishCtx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	var locked string
	err = tx.QueryRow(finishCtx, `SELECT operation_id::text FROM ops.remote_dispatches
	 WHERE operation_id=$1 AND state='DISPATCHING' AND claim_token=$2
	 AND lease_expires_at>clock_timestamp() FOR UPDATE`,
		claim.operation.OperationID, claim.token).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	operation, err := loadOpsOperation(finishCtx, tx, claim.operation.OperationID, true)
	if err != nil {
		return err
	}
	if operation.OperationType != claim.operation.OperationType ||
		operation.State != "EXECUTING" && operation.State != "RECOVERING" {
		return ErrOpsConflict
	}
	var now time.Time
	if err = tx.QueryRow(finishCtx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return err
	}
	if err = apply(finishCtx, tx, operation, now); err != nil {
		return err
	}
	return tx.Commit(finishCtx)
}

func (s *OpsService) retryRemoteDispatch(ctx context.Context, claim opsRemoteClaim, code, substate string) error {
	code = opsRemoteCode(code, "OPS_REMOTE_RETRY")
	return s.withRemoteClaim(ctx, claim, func(finishCtx context.Context, tx pgx.Tx, operation OpsOperation, now time.Time) error {
		delay := opsRemoteRetryDelay(claim.attempt)
		if _, err := tx.Exec(finishCtx, `UPDATE ops.remote_dispatches SET state='RETRY',
		 next_attempt_at=clock_timestamp()+$3::interval,claim_token=NULL,lease_expires_at=NULL,
		 failure_code=$4,updated_at=clock_timestamp() WHERE operation_id=$1 AND claim_token=$2`,
			operation.OperationID, claim.token, delay.String(), code); err != nil {
			return err
		}
		if operation.State == "RECOVERING" {
			return nil
		}
		version := operation.StateVersion + 1
		if _, err := tx.Exec(finishCtx, `UPDATE ops.admin_operations SET state='RECOVERING',
		 failure_code=$2,state_version=$3,updated_at=$4 WHERE operation_id=$1`,
			operation.OperationID, code, version, now); err != nil {
			return err
		}
		return insertOpsEvent(finishCtx, tx, operation.OperationID, "RECOVERING", substate,
			version, code, json.RawMessage(`{}`), now)
	})
}

func (s *OpsService) failRemoteDispatch(ctx context.Context, claim opsRemoteClaim, code string) error {
	code = opsRemoteCode(code, "OPS_REMOTE_FAILURE")
	return s.withRemoteClaim(ctx, claim, func(finishCtx context.Context, tx pgx.Tx, operation OpsOperation, now time.Time) error {
		if _, err := tx.Exec(finishCtx, `UPDATE ops.remote_dispatches SET state='FAILED_NO_EFFECT',
		 next_attempt_at=NULL,claim_token=NULL,lease_expires_at=NULL,failure_code=$3,
		 updated_at=$4,settled_at=$4 WHERE operation_id=$1 AND claim_token=$2`,
			operation.OperationID, claim.token, code, now); err != nil {
			return err
		}
		version := operation.StateVersion + 1
		if _, err := tx.Exec(finishCtx, `UPDATE ops.admin_operations SET state='FAILED_NO_EFFECT',
		 failure_code=$2,state_version=$3,completed_at=$4,updated_at=$4 WHERE operation_id=$1`,
			operation.OperationID, code, version, now); err != nil {
			return err
		}
		return insertOpsEvent(finishCtx, tx, operation.OperationID, "FAILED_NO_EFFECT",
			"REMOTE_REJECTED_NO_EFFECT", version, code, json.RawMessage(`{}`), now)
	})
}

func (s *OpsService) reviewRemoteDispatch(ctx context.Context, claim opsRemoteClaim, code string, receipt json.RawMessage) error {
	code = opsRemoteCode(code, "OPS_REMOTE_NEEDS_REVIEW")
	var storedReceipt any
	var receiptHash any
	result := map[string]string{"state": "NEEDS_REVIEW", "failure_code": code}
	if len(receipt) > 0 {
		digest := sha256.Sum256(append([]byte("CHALDEA-OPS-REMOTE-RECEIPT-V1\x00"), receipt...))
		storedReceipt = receipt
		receiptHash = digest[:]
		result["remote_receipt_hash"] = hex.EncodeToString(digest[:])
	}
	resultRaw, _ := json.Marshal(result)
	return s.withRemoteClaim(ctx, claim, func(finishCtx context.Context, tx pgx.Tx, operation OpsOperation, now time.Time) error {
		if _, err := tx.Exec(finishCtx, `UPDATE ops.remote_dispatches SET state='NEEDS_REVIEW',
		 next_attempt_at=NULL,claim_token=NULL,lease_expires_at=NULL,failure_code=$3,
		 receipt_payload=$4,receipt_hash=$5,updated_at=$6,settled_at=$6
		 WHERE operation_id=$1 AND claim_token=$2`, operation.OperationID, claim.token,
			code, storedReceipt, receiptHash, now); err != nil {
			return err
		}
		version := operation.StateVersion + 1
		if _, err := tx.Exec(finishCtx, `UPDATE ops.admin_operations SET state='NEEDS_REVIEW',
		 failure_code=$2,state_version=$3,result=$4,updated_at=$5 WHERE operation_id=$1`,
			operation.OperationID, code, version, resultRaw, now); err != nil {
			return err
		}
		return insertOpsEvent(finishCtx, tx, operation.OperationID, "NEEDS_REVIEW",
			"REMOTE_RECEIPT_NEEDS_REVIEW", version, code, json.RawMessage(`{}`), now)
	})
}

func (s *OpsService) acceptRemoteReview(ctx context.Context, claim opsRemoteClaim, code string,
	receipt json.RawMessage, execution OpsExecutionResult, audit normalizedOpsAudit) error {
	code = opsRemoteCode(code, "OPS_REMOTE_NEEDS_REVIEW")
	receiptDigest := sha256.Sum256(append([]byte("CHALDEA-OPS-REMOTE-RECEIPT-V1\x00"), receipt...))
	return s.withRemoteClaim(ctx, claim, func(finishCtx context.Context, tx pgx.Tx, operation OpsOperation, now time.Time) error {
		operation.RelatedBusinessID = execution.RelatedBusinessID
		if _, err := insertOpsAudit(finishCtx, tx, OpsPrincipal{UserID: operation.ActorUserID,
			Role: operation.ActorRole, Epoch: operation.ActorAuthzEpoch, Scopes: append([]string{}, operation.ActorScopes...)},
			operation, audit, now); err != nil {
			return err
		}
		if _, err := tx.Exec(finishCtx, `UPDATE ops.remote_dispatches SET state='NEEDS_REVIEW',
		 next_attempt_at=NULL,claim_token=NULL,lease_expires_at=NULL,failure_code=$3,
		 receipt_payload=$4,receipt_hash=$5,updated_at=$6,settled_at=$6
		 WHERE operation_id=$1 AND claim_token=$2`, operation.OperationID, claim.token,
			code, receipt, receiptDigest[:], now); err != nil {
			return err
		}
		version := operation.StateVersion + 1
		if _, err := tx.Exec(finishCtx, `UPDATE ops.admin_operations SET state='NEEDS_REVIEW',
		 failure_code=$2,state_version=$3,result=$4,related_business_id=NULLIF($5,''),updated_at=$6
		 WHERE operation_id=$1`, operation.OperationID, code, version, audit.Result,
			execution.RelatedBusinessID, now); err != nil {
			return err
		}
		return insertOpsEvent(finishCtx, tx, operation.OperationID, "NEEDS_REVIEW",
			"REMOTE_RECEIPT_ACCEPTED_FOR_REVIEW", version, code, json.RawMessage(`{}`), now)
	})
}

func (s *OpsService) succeedRemoteDispatch(ctx context.Context, claim opsRemoteClaim, receipt json.RawMessage,
	execution OpsExecutionResult, audit normalizedOpsAudit) error {
	receiptDigest := sha256.Sum256(append([]byte("CHALDEA-OPS-REMOTE-RECEIPT-V1\x00"), receipt...))
	return s.withRemoteClaim(ctx, claim, func(finishCtx context.Context, tx pgx.Tx, operation OpsOperation, now time.Time) error {
		operation.RelatedBusinessID = execution.RelatedBusinessID
		if _, err := insertOpsAudit(finishCtx, tx, OpsPrincipal{UserID: operation.ActorUserID,
			Role: operation.ActorRole, Epoch: operation.ActorAuthzEpoch, Scopes: append([]string{}, operation.ActorScopes...)},
			operation, audit, now); err != nil {
			return err
		}
		if _, err := tx.Exec(finishCtx, `UPDATE ops.remote_dispatches SET state='SUCCEEDED',
		 next_attempt_at=NULL,claim_token=NULL,lease_expires_at=NULL,failure_code=NULL,
		 receipt_payload=$3,receipt_hash=$4,updated_at=$5,settled_at=$5
		 WHERE operation_id=$1 AND claim_token=$2`, operation.OperationID, claim.token,
			receipt, receiptDigest[:], now); err != nil {
			return err
		}
		version := operation.StateVersion + 1
		if _, err := tx.Exec(finishCtx, `UPDATE ops.admin_operations SET state='SUCCEEDED',
		 failure_code=NULL,state_version=$2,result=$3,related_business_id=NULLIF($4,''),
		 completed_at=$5,updated_at=$5 WHERE operation_id=$1`, operation.OperationID,
			version, audit.Result, execution.RelatedBusinessID, now); err != nil {
			return err
		}
		return insertOpsEvent(finishCtx, tx, operation.OperationID, "SUCCEEDED",
			"REMOTE_RECEIPT_ACCEPTED", version, "", json.RawMessage(`{}`), now)
	})
}

// RunOpsRemoteWorker is the default joined lifecycle loop. Callers needing
// health observations can drive RunRemoteDispatchStep directly instead.
func RunOpsRemoteWorker(ctx context.Context, service *OpsService) {
	if service == nil {
		return
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			stepCtx, cancel := context.WithTimeout(ctx, opsRemoteStepTimeout)
			worked, _ := service.RunRemoteDispatchStep(stepCtx)
			cancel()
			delay := opsRemoteIdleInterval
			if worked {
				delay = 0
			}
			timer.Reset(delay)
		}
	}
}
