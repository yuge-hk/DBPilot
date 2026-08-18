package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig `yaml:"server"`
	DataDir string       `yaml:"data_dir"`
	LogDir  string       `yaml:"log_dir"`
	Log     LogConfig    `yaml:"log"`
}

type ServerConfig struct {
	Addr          string `yaml:"addr"`
	Port          int    `yaml:"port"`
	SessionSecret string `yaml:"session_secret"`
	ExternalURL   string `yaml:"external_url"`
}

type LogConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Addr: "127.0.0.1",
			Port: 8080,
		},
		DataDir: "/var/lib/dbpilot",
		LogDir:  "/var/log/dbpilot",
		Log: LogConfig{
			Level: "info",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "dbpilot.db")
}

func (c *Config) TmpDir() string {
	return filepath.Join(c.DataDir, "tmp")
}

func (c *Config) BackupsDir() string {
	return filepath.Join(c.DataDir, "backups")
}

func (c *Config) KeyPath() string {
	return filepath.Join(c.DataDir, "secret.key")
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Addr, c.Server.Port)
}
