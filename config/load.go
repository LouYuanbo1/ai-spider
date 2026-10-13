package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Rod   *RodConfig   `yaml:"rod"`
	Model *ModelConfig `yaml:"model"`
}

// Load 读取 YAML 配置文件，返回 rod 配置与登录账号
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
