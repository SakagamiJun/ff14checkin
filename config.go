package main

import (
	"encoding/json"
	"os"
)

type TaskConfig struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	CookieStr string `json:"cookie_str"`
}

type Config struct {
	Tasks []TaskConfig `json:"tasks"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Tasks: []TaskConfig{}}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveConfig(filename string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// 使用 0600 权限：仅当前用户读写，防止其它用户窃取 Cookie
	return os.WriteFile(filename, data, 0600)
}
func (c *Config) GetTask(name string) *TaskConfig {
	for i := range c.Tasks {
		if c.Tasks[i].Name == name {
			return &c.Tasks[i]
		}
	}
	return nil
}

func (c *Config) UpdateTaskCookie(name, cookieStr string) {
	for i := range c.Tasks {
		if c.Tasks[i].Name == name {
			c.Tasks[i].CookieStr = cookieStr
			return
		}
	}
	// 如果不存在，则新增（通常用于初始化）
	c.Tasks = append(c.Tasks, TaskConfig{Name: name, CookieStr: cookieStr})
}