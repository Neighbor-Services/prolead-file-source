package service

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
)

type AuditService struct {
	repo    *sqlite.AuditRepo
	logChan chan *domain.AuditLog
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewAuditService(repo *sqlite.AuditRepo) *AuditService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &AuditService{
		repo:    repo,
		logChan: make(chan *domain.AuditLog, 2048),
		ctx:     ctx,
		cancel:  cancel,
	}

	go s.worker()
	return s
}

func (s *AuditService) worker() {
	for {
		select {
		case <-s.ctx.Done():
			// Flush remaining
			for len(s.logChan) > 0 {
				item := <-s.logChan
				_ = s.repo.Create(item)
			}
			return
		case item := <-s.logChan:
			if err := s.repo.Create(item); err != nil {
				log.Printf("⚠️ Failed to write audit log: %v", err)
			}
		}
	}
}

func (s *AuditService) Record(action, bucket, path, actor, ip, userAgent string, status int, durationMs int64) {
	entry := &domain.AuditLog{
		ID:         uuid.New().String(),
		Action:     action,
		Bucket:     bucket,
		Path:       path,
		Actor:      actor,
		IPAddress:  ip,
		UserAgent:  userAgent,
		Status:     status,
		DurationMs: durationMs,
		CreatedAt:  time.Now().UTC(),
	}

	select {
	case s.logChan <- entry:
	default:
		// Drop if buffer full to avoid blocking critical path
		log.Printf("⚠️ Audit buffer full, dropping audit log for %s %s/%s", action, bucket, path)
	}
}

func (s *AuditService) List(filter sqlite.AuditFilter) ([]domain.AuditLog, int64, error) {
	return s.repo.List(filter)
}

func (s *AuditService) Close() {
	s.cancel()
}
