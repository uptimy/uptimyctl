package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	DefaultAPIURL  = "https://api.upti.my"
	ConfigFileName = ".uptimyctl"
)

type Config struct {
	APIURL string `mapstructure:"api_url"`
	APIKey string `mapstructure:"api_key"`
}

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".config", "uptimyctl")
}

func configPath() string {
	return filepath.Join(configDir(), "config.yaml")
}

func Load() *Config {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configDir())
	viper.SetDefault("api_url", DefaultAPIURL)
	viper.SetDefault("api_key", "")
	viper.SetEnvPrefix("UPTIMYCTL")
	viper.AutomaticEnv()

	var cfg Config
	_ = viper.ReadInConfig()
	_ = viper.Unmarshal(&cfg)
	return &cfg
}

func Save(cfg *Config) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	viper.Set("api_url", cfg.APIURL)
	viper.Set("api_key", cfg.APIKey)
	return viper.WriteConfigAs(configPath())
}

func GetAPIKey(flagOverride string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if envKey := os.Getenv("UPTIMYCTL_API_KEY"); envKey != "" {
		return envKey
	}
	return Load().APIKey
}

func GetAPIURL(flagOverride string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if envURL := os.Getenv("UPTIMYCTL_API_URL"); envURL != "" {
		return envURL
	}
	cfg := Load()
	if cfg.APIURL != "" {
		return cfg.APIURL
	}
	return DefaultAPIURL
}
