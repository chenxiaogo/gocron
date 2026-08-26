package cache

import "time"

// Config is the cache configuration.
type Config struct {
	Name        string        `yaml:"name" json:"name"`
	Namespace   string        `yaml:"namespace" json:"namespace"`
	DefaultTTL  time.Duration `yaml:"default_ttl" json:"default_ttl"`
	NotFoundTTL time.Duration `yaml:"not_found_ttl" json:"not_found_ttl"`

	Local struct {
		Enabled     bool          `yaml:"enabled" json:"enabled"`
		Type        string        `yaml:"type" json:"type"`
		Size        int64         `yaml:"size" json:"size"`
		TTL         time.Duration `yaml:"ttl" json:"ttl"`
		CleanWindow time.Duration `yaml:"clean_window" json:"clean_window"`
	} `yaml:"local" json:"local"`

	Remote struct {
		Enabled     bool          `yaml:"enabled" json:"enabled"`
		Type        string        `yaml:"type" json:"type"`
		Addresses   []string      `yaml:"addresses" json:"addresses"`
		Password    string        `yaml:"password" json:"password"`
		DB          int           `yaml:"db" json:"db"`
		PoolSize    int           `yaml:"pool_size" json:"pool_size"`
		MaxRetries  int           `yaml:"max_retries" json:"max_retries"`
		DialTimeout time.Duration `yaml:"dial_timeout" json:"dial_timeout"`
	} `yaml:"remote" json:"remote"`

	RefreshInterval time.Duration `yaml:"refresh_interval" json:"refresh_interval"`
	EnableStats     bool          `yaml:"enable_stats" json:"enable_stats"`
	MonitorType     string        `yaml:"monitor_type" json:"monitor_type"`
	LogLevel        string        `yaml:"log_level" json:"log_level"`

	// Logger is an external logger (optional).
	Logger Logger `yaml:"-" json:"-"`
}

// DefaultConfig returns a default cache configuration.
func DefaultConfig() *Config {
	return &Config{
		Name:        "scheduler-cache",
		Namespace:   "scheduler",
		DefaultTTL:  time.Hour,
		NotFoundTTL: time.Minute * 5,
		Local: struct {
			Enabled     bool          `yaml:"enabled" json:"enabled"`
			Type        string        `yaml:"type" json:"type"`
			Size        int64         `yaml:"size" json:"size"`
			TTL         time.Duration `yaml:"ttl" json:"ttl"`
			CleanWindow time.Duration `yaml:"clean_window" json:"clean_window"`
		}{
			Enabled:     false,
			Type:        "freecache",
			Size:        256 * 1024 * 1024, // 256MB
			TTL:         time.Hour,
			CleanWindow: time.Minute * 5,
		},
		Remote: struct {
			Enabled     bool          `yaml:"enabled" json:"enabled"`
			Type        string        `yaml:"type" json:"type"`
			Addresses   []string      `yaml:"addresses" json:"addresses"`
			Password    string        `yaml:"password" json:"password"`
			DB          int           `yaml:"db" json:"db"`
			PoolSize    int           `yaml:"pool_size" json:"pool_size"`
			MaxRetries  int           `yaml:"max_retries" json:"max_retries"`
			DialTimeout time.Duration `yaml:"dial_timeout" json:"dial_timeout"`
		}{
			Enabled:     false, // 默认仅本地缓存，不连 Redis
			Type:        "redis",
			Addresses:   []string{"localhost:6379"},
			PoolSize:    100,
			MaxRetries:  3,
			DialTimeout: time.Second * 5,
		},
		RefreshInterval: time.Minute * 30,
		EnableStats:     true,
		LogLevel:        "info",
	}
}