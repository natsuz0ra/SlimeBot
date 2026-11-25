package repositories

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"slimebot/internal/domain"
)

func contextCalibrationID(provider, model string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(model)))
	return hex.EncodeToString(sum[:])
}

// LoadContextCalibration reads a provider/model estimate without creating state.
func (r *Repository) LoadContextCalibration(ctx context.Context, provider, model string) (domain.ContextCalibration, bool, error) {
	var calibration domain.ContextCalibration
	err := r.dbWithContext(ctx).Where("provider = ? AND model = ?", strings.TrimSpace(provider), strings.TrimSpace(model)).Take(&calibration).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ContextCalibration{}, false, nil
	}
	return calibration, err == nil, err
}

// SaveContextCalibration updates the latest conservative ratio and sample count.
func (r *Repository) SaveContextCalibration(ctx context.Context, calibration *domain.ContextCalibration) error {
	calibration.ID = contextCalibrationID(calibration.Provider, calibration.Model)
	calibration.UpdatedAt = time.Now()
	return r.dbWithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "provider"}, {Name: "model"}},
		DoUpdates: clause.Assignments(map[string]any{
			"ratio":      gorm.Expr("MAX(context_calibrations.ratio, excluded.ratio)"),
			"samples":    gorm.Expr("context_calibrations.samples + 1"),
			"updated_at": calibration.UpdatedAt,
		}),
	}).Create(calibration).Error
}
