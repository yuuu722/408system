package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"time"
)

var ErrStaleExecution = errors.New("处理执行已失效（Stale execution）")

func (p *Postgres) EnqueueDocument(ctx context.Context, uid, did string) error {
	if e := p.CanMaintainDocument(ctx, uid, did); e != nil {
		return e
	}
	d, e := p.Document(ctx, uid, did)
	if e != nil {
		return e
	}
	uid = d.UserID
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, `UPDATE documents SET status='processing',processing_stage='queued',progress=0,processed_chunks=0,total_chunks=0,error_message='',updated_at=now() WHERE id=$1 AND user_id=$2 AND status<>'processing'`, did, uid)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, e = tx.Exec(ctx, `INSERT INTO document_jobs(document_id) VALUES($1) ON CONFLICT(document_id) DO UPDATE SET state='queued',attempts=0,last_error='',available_at=now(),lease_until=NULL,execution=document_jobs.execution+1,updated_at=now()`, did)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// Short transactions own a job. Every worker write is fenced by its execution number.
func (p *Postgres) RunDocumentJob(ctx context.Context, process func(context.Context, domain.Document) error) (bool, error) {
	_, e := p.DB.Exec(ctx, `WITH expired AS (UPDATE document_jobs SET state='failed',lease_until=NULL,last_error='处理多次中断，请重试（Repeated interruption）' WHERE state='running' AND lease_until<now() AND attempts>=3 RETURNING document_id) UPDATE documents SET status='failed',error_message='处理多次中断，请重试（Repeated interruption）' WHERE id IN(SELECT document_id FROM expired)`)
	if e != nil {
		return false, e
	}
	var did, uid string
	var execution int64
	e = p.DB.QueryRow(ctx, `WITH next AS (SELECT document_id FROM document_jobs WHERE (state='queued' AND available_at<=now() OR state='running' AND lease_until<now()) AND attempts<3 ORDER BY available_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE document_jobs j SET state='running',execution=execution+1,attempts=attempts+1,lease_until=now()+interval '90 seconds',updated_at=now() FROM next n,documents d WHERE j.document_id=n.document_id AND d.id=j.document_id RETURNING j.document_id,d.user_id,j.execution`).Scan(&did, &uid, &execution)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	d, e := p.Document(ctx, uid, did)
	if e != nil {
		return true, e
	}
	d.Execution = execution
	timeout := p.DocumentJobTimeout
	if timeout <= 0 {
		timeout = 2 * time.Hour
	}
	work, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-work.Done():
				return
			case <-tick.C:
				tag, err := p.DB.Exec(work, `UPDATE document_jobs SET lease_until=now()+interval '90 seconds',updated_at=now() WHERE document_id=$1 AND execution=$2 AND state='running' AND lease_until>now()`, did, execution)
				if err != nil || tag.RowsAffected() != 1 {
					cancel()
					return
				}
			}
		}
	}()
	processErr := process(work, d)
	close(stop)
	<-stopped
	finish, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer done()
	if processErr == nil {
		var committed bool
		if e = p.DB.QueryRow(finish, `SELECT EXISTS(SELECT 1 FROM document_jobs WHERE document_id=$1 AND execution=$2 AND state='completed')`, did, execution).Scan(&committed); e != nil {
			return true, e
		}
		if committed {
			return true, nil
		}
	}
	tx, e := p.executionTx(finish, d)
	if e != nil {
		return true, e
	}
	defer tx.Rollback(finish)
	state, msg := "completed", ""
	retry := false
	if ctx.Err() != nil {
		state = "queued"
		msg = "处理已暂停，可继续（Interrupted）"
		retry = true
	} else if processErr != nil {
		state = "failed"
		msg = processErr.Error()
		retry = retryableDocumentError(processErr)
		if retry {
			state = "queued"
		}
	}
	var attempts int
	if e = tx.QueryRow(finish, `SELECT attempts FROM document_jobs WHERE document_id=$1`, did).Scan(&attempts); e != nil {
		return true, e
	}
	if attempts >= 3 && ctx.Err() == nil && state == "queued" {
		state = "failed"
		retry = false
	}
	delay := attempts * 10
	if ctx.Err() != nil {
		attempts--
		delay = 0
	}
	_, e = tx.Exec(finish, `UPDATE document_jobs SET state=$2,attempts=$3,last_error=$4,lease_until=NULL,available_at=now()+($5*interval '1 second'),updated_at=now() WHERE document_id=$1`, did, state, attempts, msg, delay)
	if e != nil {
		return true, e
	}
	if processErr != nil || ctx.Err() != nil {
		status, stage := "failed", "failed"
		if retry {
			status, stage = "processing", "retry_wait"
		}
		_, e = tx.Exec(finish, `UPDATE documents SET status=$2,processing_stage=$3,error_message=$4,updated_at=now() WHERE id=$1`, did, status, stage, msg)
		if e != nil {
			return true, e
		}
	}
	if e = tx.Commit(finish); e != nil {
		return true, e
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	return true, nil
}

func (p *Postgres) executionTx(ctx context.Context, d domain.Document) (pgx.Tx, error) {
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	var valid bool
	e = tx.QueryRow(ctx, `SELECT state='running' AND execution=$2 AND lease_until>now() FROM document_jobs WHERE document_id=$1 FOR UPDATE`, d.ID, d.Execution).Scan(&valid)
	if e != nil || !valid {
		tx.Rollback(ctx)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return nil, e
		}
		return nil, ErrStaleExecution
	}
	return tx, nil
}
func retryableDocumentError(err error) bool {
	var t interface{ Temporary() bool }
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &t) && t.Temporary())
}
