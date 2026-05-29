package main

import (
	"encoding/json"
	"os"
)

type Cookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

type TaskConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Config struct {
	Cookies      []Cookie     `json:"cookies"`
	Tasks        []TaskConfig `json:"tasks"`
	DeviceId     string       `json:"deviceId,omitempty"`
	MacId        string       `json:"macId,omitempty"`
	AutoLoginKey string       `json:"autoLoginKey,omitempty"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Tasks: []TaskConfig{}, Cookies: []Cookie{}}, nil
		}
		return nil, err
	}

	// 兼容旧配置：如果 cookies 在 task 里面，提取并合并到全局
	type OldTaskConfig struct {
		Name    string   `json:"name"`
		URL     string   `json:"url"`
		Cookies []Cookie `json:"cookies"`
	}
	type OldConfig struct {
		Tasks []OldTaskConfig `json:"tasks"`
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if len(cfg.Cookies) == 0 {
		var oldCfg OldConfig
		if err := json.Unmarshal(data, &oldCfg); err == nil {
			cookieMap := make(map[string]Cookie)
			for _, t := range oldCfg.Tasks {
				for _, c := range t.Cookies {
					key := c.Name + "|" + c.Domain + "|" + c.Path
					cookieMap[key] = c
				}
			}
			for _, c := range cookieMap {
				cfg.Cookies = append(cfg.Cookies, c)
			}
			if len(cfg.Cookies) > 0 {
				SaveConfig(filename, &cfg)
			}
		}
	}

	return &cfg, nil
}

func SaveConfig(filename string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
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
