package models

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Config 结构包含应用程序的所有配置信息
type Config struct {
	ARK struct {
		APIKey    string `json:"api_key"`
		ModelName string `json:"model_name"`
	} `json:"ark"`
}

var (
	config     *Config
	configOnce sync.Once
	configErr  error
)

// LoadConfig 从配置文件加载配置
func LoadConfig() (*Config, error) {
	configOnce.Do(func() {
		// 优先从环境变量获取配置文件路径
		configPath := os.Getenv("CONFIG_PATH")
		if configPath == "" {
			configPath = "config/config.json" // 默认配置文件路径
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			configErr = fmt.Errorf("读取配置文件失败: %v", err)
			return
		}

		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			configErr = fmt.Errorf("解析配置文件失败: %v", err)
			return
		}

		// 检查必要的配置是否存在
		if cfg.ARK.APIKey == "" {
			configErr = fmt.Errorf("ARK API密钥未配置")
			return
		}

		config = &cfg
	})

	return config, configErr
}

// GetARKAPIKey 获取ARK API密钥
func GetARKAPIKey() (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}
	return cfg.ARK.APIKey, nil
}

// GetARKModelName 获取ARK模型名称
func GetARKModelName() (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}
	return cfg.ARK.ModelName, nil
}
