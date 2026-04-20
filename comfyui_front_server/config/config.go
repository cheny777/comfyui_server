package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

var globalConfig *Config

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	User     UserConfig     `yaml:"user"`
	Middle   MiddleConfig   `yaml:"middle"`
	Storage  StorageConfig  `yaml:"storage"`
	Log      LogConfig      `yaml:"log"`
}

type ServerConfig struct {
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
	WSPath string `yaml:"ws_path"`
}

type DatabaseConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"`
}

type UserConfig struct {
	DefaultUserType   string `yaml:"default_user_type"`
	CleanupInactiveDays int  `yaml:"cleanup_inactive_days"`
}

type MiddleConfig struct {
	SecretKey string `yaml:"secret_key"`
}

type StorageConfig struct {
	ImageBaseURL string `yaml:"image_base_url"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	globalConfig = &config
	return &config, nil
}

func GetConfig() *Config {
	return globalConfig
}

func GetJWTSecret() string {
	if globalConfig == nil {
		return "default_secret_key"
	}
	return globalConfig.JWT.Secret
}

func GetJWTExpireDuration() time.Duration {
	if globalConfig == nil {
		return 720 * time.Hour // 默认30天
	}
	return time.Duration(globalConfig.JWT.ExpireHours) * time.Hour
}

func GetMiddleSecretKey() string {
	if globalConfig == nil {
		return "default_middle_secret_key"
	}
	return globalConfig.Middle.SecretKey
}

