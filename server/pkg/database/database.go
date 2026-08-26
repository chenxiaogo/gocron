package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"scheduler-platform/pkg/app/config"
)

// New 根据配置初始化 GORM，driver 支持 sqlite / mysql。
func New(conf *config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch strings.ToLower(conf.Driver) {
	case "mysql":
		dialector = mysql.Open(conf.DSN)
	default: // sqlite
		dialector = sqlite.Open(conf.DSN)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: NewGormLogger(),
	})
	if err != nil {
		return nil, fmt.Errorf("open database(%s): %w", conf.Driver, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxIdleConns(conf.MaxIdleConns)
	sqlDB.SetMaxOpenConns(conf.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(conf.ConnMaxLifetime)

	return db, nil
}

// InitMaxIdleConns 默认连接池配置（供无配置时使用）。
const (
	DefaultMaxIdleConns  = 10
	DefaultMaxOpenConns  = 100
	DefaultConnMaxLifetime = 2 * time.Hour
)
