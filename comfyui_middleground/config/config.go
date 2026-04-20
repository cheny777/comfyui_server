package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	FrontServer FrontServerConfig `yaml:"front_server"`
	ComfyUI     ComfyUIConfig     `yaml:"comfyui"`
	Storage     StorageConfig     `yaml:"storage"`
	Database    DatabaseConfig    `yaml:"database"`
	Server      ServerConfig      `yaml:"server"`
	Log         LogConfig         `yaml:"log"`
}

type FrontServerConfig struct {
	WSURL             string        `yaml:"ws_url"`
	ServerID          string        `yaml:"server_id"`
	SecretKey         string        `yaml:"secret_key"`
	ReconnectInterval time.Duration `yaml:"reconnect_interval"`
}

type ComfyUIConfig struct {
	Host        string        `yaml:"host"`
	HTTPTimeout time.Duration `yaml:"http_timeout"`
}

type StorageConfig struct {
	ImagePath          string `yaml:"image_path"`
	MaxConcurrentTasks int    `yaml:"max_concurrent_tasks"`
}

type DatabaseConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

var GlobalConfig *Config

func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	GlobalConfig = &config
	return &config, nil
}

func GetConfig() *Config {
	if GlobalConfig == nil {
		// 返回默认配置
		return &Config{
			FrontServer: FrontServerConfig{
				WSURL:             "ws://127.0.0.1:8080/ws/middle",
				ServerID:          "middle_server_001",
				SecretKey:         "your_middle_secret_key",
				ReconnectInterval: 10 * time.Second,
			},
			ComfyUI: ComfyUIConfig{
				Host:        "127.0.0.1:8188",
				HTTPTimeout: 30 * time.Second,
			},
			Storage: StorageConfig{
				ImagePath:          "./data/images",
				MaxConcurrentTasks: 5,
			},
			Database: DatabaseConfig{
				Type: "sqlite",
				Path: "./data/middle.db",
			},
			Server: ServerConfig{
				Host: "0.0.0.0",
				Port: 8081,
			},
			Log: LogConfig{
				Level: "INFO",
			},
		}
	}
	return GlobalConfig
}
