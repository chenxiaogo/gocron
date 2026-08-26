package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

var (
	_config   *config
	_configMu sync.RWMutex
)

const (
	defaultConfigName = "config.yaml"
	defaultConfigPath = "conf.d/config.yaml"
)

// ConfigUpdate 配置更新通知。
type ConfigUpdate struct {
	Config *Config
	Error  error
	Time   time.Time
}

type config struct {
	cfg         *Config
	cfgChangeCh chan ConfigUpdate
	watchOnce   sync.Once
	loadOnce    sync.Once
	mu          sync.RWMutex
	stopCh      chan struct{}
	viper       *viper.Viper
}

// Config 应用配置。
type Config struct {
	App      AppConfig      `json:"app" yaml:"app" mapstructure:"app"`
	Server   ServerConfig   `json:"server" yaml:"server" mapstructure:"server"`
	Database DatabaseConfig `json:"database" yaml:"database" mapstructure:"database"`
	Security SecurityConfig `json:"security" yaml:"security" mapstructure:"security"`
	Logger   LoggerConfig   `json:"logger" yaml:"logger" mapstructure:"logger"`
	Metrics  MetricsConfig  `json:"metrics" yaml:"metrics" mapstructure:"metrics"`
	Cache    CacheConfig    `json:"cache" yaml:"cache" mapstructure:"cache"`
	LastUp   time.Time      `json:"-" yaml:"-"`
}

type AppConfig struct {
	Name string `json:"name" yaml:"name" mapstructure:"name"`
	Env  string `json:"env" yaml:"env" mapstructure:"env"`
}

type ServerConfig struct {
	Host string `json:"host" yaml:"host" mapstructure:"host"`
	Port int    `json:"port" yaml:"port" mapstructure:"port"`
}

type DatabaseConfig struct {
	Driver          string        `json:"driver" yaml:"driver" mapstructure:"driver"`
	DSN             string        `json:"dsn" yaml:"dsn" mapstructure:"dsn"`
	MaxIdleConns    int           `json:"maxIdleConns" yaml:"maxIdleConns" mapstructure:"maxIdleConns"`
	MaxOpenConns    int           `json:"maxOpenConns" yaml:"maxOpenConns" mapstructure:"maxOpenConns"`
	ConnMaxLifetime time.Duration `json:"connMaxLifetime" yaml:"connMaxLifetime" mapstructure:"connMaxLifetime"`
}

type SecurityConfig struct {
	JWTSecret        string `json:"jwtSecret" yaml:"jwtSecret" mapstructure:"jwtSecret"`
	TokenExpireHours int    `json:"tokenExpireHours" yaml:"tokenExpireHours" mapstructure:"tokenExpireHours"`
}

type LoggerConfig struct {
	Level    string `json:"level" yaml:"level" mapstructure:"level"`
	FilePath string `json:"filePath" yaml:"filePath" mapstructure:"filePath"`
	MaxSize  int    `json:"maxSize" yaml:"maxSize" mapstructure:"maxSize"`
}

// MetricsConfig Prometheus 指标配置。
type MetricsConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	Path    string `json:"path" yaml:"path" mapstructure:"path"`
}

// CacheConfig 缓存配置（基于 jetcache-go，支持本地+远程二级缓存）。
type CacheConfig struct {
	Enabled       bool          `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	DefaultTTL    time.Duration `json:"defaultTtl" yaml:"defaultTtl" mapstructure:"defaultTtl"`
	NotFoundTTL   time.Duration `json:"notFoundTtl" yaml:"notFoundTtl" mapstructure:"notFoundTtl"`
	LocalEnabled  bool          `json:"localEnabled" yaml:"localEnabled" mapstructure:"localEnabled"`
	LocalSize     int64         `json:"localSize" yaml:"localSize" mapstructure:"localSize"`
	LocalTTL      time.Duration `json:"localTtl" yaml:"localTtl" mapstructure:"localTtl"`
	RedisAddr     string        `json:"redisAddr" yaml:"redisAddr" mapstructure:"redisAddr"`
	RedisPassword string        `json:"redisPassword" yaml:"redisPassword" mapstructure:"redisPassword"`
	RedisDB       int           `json:"redisDb" yaml:"redisDb" mapstructure:"redisDb"`
	RedisPoolSize int           `json:"redisPoolSize" yaml:"redisPoolSize" mapstructure:"redisPoolSize"`
	DialTimeout   time.Duration `json:"dialTimeout" yaml:"dialTimeout" mapstructure:"dialTimeout"`
}

// New 创建默认配置。
func New() *Config {
	return &Config{
		App: AppConfig{Name: "scheduler-platform", Env: "dev"},
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Database: DatabaseConfig{
			Driver:          "sqlite",
			DSN:             "scheduler.db",
			MaxIdleConns:    10,
			MaxOpenConns:    100,
			ConnMaxLifetime: 2 * time.Hour,
		},
		Security: SecurityConfig{JWTSecret: "scheduler-platform-secret"},
		Logger: LoggerConfig{
			Level:    "info",
			FilePath: "./logs",
			MaxSize:  100,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		Cache: CacheConfig{
			Enabled:       true,
			DefaultTTL:    10 * time.Minute,
			NotFoundTTL:   2 * time.Minute,
			LocalEnabled:  true,
			LocalSize:     256 * 1024 * 1024,
			LocalTTL:      3 * time.Minute,
			RedisAddr:     "",
			RedisPassword: "",
			RedisDB:       0,
			RedisPoolSize: 0,
			DialTimeout:   5 * time.Second,
		},
		LastUp: time.Now(),
	}
}

func (c *Config) DeepCopy() *Config {
	if c == nil {
		return New()
	}
	cp := *c
	return &cp
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Security.JWTSecret == "" {
		return fmt.Errorf("jwt secret cannot be empty")
	}
	return nil
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func defaultConfig(path string) *config {
	if path == "" {
		path = defaultConfigPath
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("SCHED")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	return &config{
		cfg:         New(),
		cfgChangeCh: make(chan ConfigUpdate, 5),
		stopCh:      make(chan struct{}),
		viper:       v,
	}
}

func (c *config) loadFromDisk() (*Config, error) {
	var err error
	c.loadOnce.Do(func() {
		if c.viper == nil {
			err = fmt.Errorf("viper not initialized")
			return
		}
		if readErr := c.viper.ReadInConfig(); readErr != nil {
			if _, ok := readErr.(viper.ConfigFileNotFoundError); ok {
				c.cfg = New()
				err = nil
				return
			}
			err = readErr
			return
		}
		temp := New()
		if unmarshalErr := c.viper.Unmarshal(temp); unmarshalErr != nil {
			err = unmarshalErr
			return
		}
		temp.LastUp = time.Now()
		if vErr := temp.Validate(); vErr != nil {
			err = vErr
			return
		}
		c.cfg = temp
	})
	return c.cfg.DeepCopy(), err
}

func (c *config) GetConfig() *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg.DeepCopy()
}

// TryLoadFromDisk 加载配置。
func TryLoadFromDisk(path string) (*Config, error) {
	_configMu.Lock()
	defer _configMu.Unlock()
	if _config != nil {
		return _config.GetConfig(), nil
	}
	_config = defaultConfig(path)
	cfg, err := _config.loadFromDisk()
	if err != nil {
		_config = nil
		return nil, err
	}
	return cfg, nil
}

// GetCurrentConfig 获取当前配置副本。
func GetCurrentConfig() *Config {
	_configMu.RLock()
	defer _configMu.RUnlock()
	if _config == nil {
		return New()
	}
	return _config.GetConfig()
}

// WatchConfigChange 监听配置变化。
func WatchConfigChange() <-chan ConfigUpdate {
	_configMu.RLock()
	defer _configMu.RUnlock()
	if _config == nil {
		ch := make(chan ConfigUpdate)
		close(ch)
		return ch
	}
	_config.watchOnce.Do(func() {
		if _config.viper == nil {
			return
		}
		_config.viper.WatchConfig()
		_config.viper.OnConfigChange(func(e fsnotify.Event) {
			select {
			case <-_config.stopCh:
				return
			default:
			}
			temp := New()
			if err := _config.viper.Unmarshal(temp); err != nil {
				_config.send(ConfigUpdate{Error: err, Time: time.Now()})
				return
			}
			if err := temp.Validate(); err != nil {
				_config.send(ConfigUpdate{Error: err, Time: time.Now()})
				return
			}
			temp.LastUp = time.Now()
			_config.mu.Lock()
			_config.cfg = temp
			_config.mu.Unlock()
			_config.send(ConfigUpdate{Config: temp.DeepCopy(), Time: time.Now()})
		})
	})
	return _config.cfgChangeCh
}

func (c *config) send(u ConfigUpdate) {
	select {
	case c.cfgChangeCh <- u:
	default:
	}
}

// StopWatching 停止配置监听。
func StopWatching() {
	_configMu.Lock()
	defer _configMu.Unlock()
	if _config != nil {
		select {
		case <-_config.stopCh:
		default:
			close(_config.stopCh)
		}
	}
}

// EnsureConfigFile 确保默认配置文件存在。
func EnsureConfigFile() error {
	if _, err := os.Stat(defaultConfigPath); err == nil {
		return nil
	}
	os.MkdirAll("conf.d", 0755)
	return os.WriteFile(defaultConfigPath, []byte(defaultYAML), 0644)
}

const defaultYAML = `app:
  name: scheduler-platform
  env: dev

server:
  host: 0.0.0.0
  port: 8080

database:
  driver: sqlite
  dsn: scheduler.db
  maxIdleConns: 10
  maxOpenConns: 100
  connMaxLifetime: 2h

security:
  jwtSecret: scheduler-platform-secret
  tokenExpireHours: 24

logger:
  level: info
  filePath: ./logs
  maxSize: 100

# Prometheus 指标暴露
metrics:
  enabled: true
  path: /metrics

# 缓存配置（本地 freecache，如需 Redis 远程缓存则设置 redisAddr）
cache:
  enabled: true           # 是否启用缓存
  defaultTtl: 10m         # 远程缓存默认过期时间
  notFoundTtl: 2m         # 空值缓存过期时间（防穿透）
  localEnabled: true      # 是否启用本地 freecache（进程内，零网络开销）
  localSize: 268435456    # 本地缓存容量（字节），256MB
  localTtl: 3m            # 本地缓存过期时间
  redisAddr: ""           # 留空则仅本地缓存；填入地址启用远程 Redis
  redisPassword: ""
  redisDb: 0
  redisPoolSize: 0
  dialTimeout: 5s
`
