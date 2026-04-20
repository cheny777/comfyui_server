package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(dbPath string) error {
	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建数据库目录失败: %w", err)
	}

	// 使用纯 Go SQLite 驱动（github.com/glebarez/sqlite）
	// 该驱动基于 modernc.org/sqlite，不需要 CGO，支持跨平台编译
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 生产环境可以设置为logger.Info
	})
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}

	// 获取底层sql.DB设置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 启用外键约束（SQLite需要）
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("启用外键约束失败: %w", err)
	}

	DB = db
	return nil
}

// AutoMigrate 自动迁移表结构，需要在InitDB之后调用
func AutoMigrate(models ...interface{}) error {
	return DB.AutoMigrate(models...)
}

func CloseDB() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
