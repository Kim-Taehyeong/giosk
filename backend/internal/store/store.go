// Package store는 DB 연결과 스키마 마이그레이션을 담당한다.
package store

import (
	"giosk/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open은 설정의 DSN으로 GORM(MySQL) 연결을 연다.
func Open(cfg config.Database) (*gorm.DB, error) {
	return gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
}
