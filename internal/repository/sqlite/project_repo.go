package sqlite

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gostore/internal/core/domain"
)

type ProjectRepository struct {
	db *DB
}

func NewProjectRepository(db *DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) Create(ctx context.Context, p *domain.Project) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	p.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *ProjectRepository) List(ctx context.Context) ([]domain.Project, error) {
	var projects []domain.Project
	err := r.db.WithContext(ctx).Order("created_at ASC").Find(&projects).Error
	return projects, err
}

func (r *ProjectRepository) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	var p domain.Project
	err := r.db.WithContext(ctx).Where("id = ? OR slug = ?", id, id).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *ProjectRepository) Delete(ctx context.Context, id string) error {
	if id == "p-default" || id == "default" {
		return errors.New("cannot delete default project")
	}
	return r.db.WithContext(ctx).Where("id = ? OR slug = ?", id, id).Delete(&domain.Project{}).Error
}
