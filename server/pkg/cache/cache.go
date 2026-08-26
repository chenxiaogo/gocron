package cache

import (
	"context"
	"fmt"
	"time"
)

// Cache is the global cache interface.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)

	MGet(ctx context.Context, keys []string) (map[string][]byte, error)
	MSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error
	MDelete(ctx context.Context, keys []string) error

	GetOrSet(ctx context.Context, key string, loader Loader, ttl time.Duration) ([]byte, error)
	GetAndDelete(ctx context.Context, key string) ([]byte, error)
	GetAndSet(ctx context.Context, key string, value []byte, ttl time.Duration) ([]byte, error)
	Increment(ctx context.Context, key string, delta int64) (int64, error)
	Decrement(ctx context.Context, key string, delta int64) (int64, error)

	TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, key string) error

	Stats() *CacheStats
	ClearStats()

	Clear(ctx context.Context) (int64, error)
	ClearByClientId(ctx context.Context, clientId int) (int64, error)
	Close() error
}

// TypedCache is a generic typed cache interface.
type TypedCache[T any] interface {
	Get(ctx context.Context, key string) (*T, error)
	Set(ctx context.Context, key string, value *T, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	GetOrSet(ctx context.Context, key string, loader TypedLoader[T], ttl time.Duration) (*T, error)
}

// Loader is a function that loads data when cache misses.
type Loader func(ctx context.Context, key string) ([]byte, error)

// TypedLoader is a typed version of Loader.
type TypedLoader[T any] func(ctx context.Context, key string) (*T, error)

// CacheStats holds cache statistics.
type CacheStats struct {
	Hits          int64
	Misses        int64
	LocalHits     int64
	RemoteHits    int64
	TotalOps      int64
	TotalHits     int64
	TotalMisses   int64
	TotalDuration time.Duration
	StartTime     time.Time
}

func (s *CacheStats) HitRate() float64 {
	if s.TotalOps == 0 {
		return 0
	}
	return float64(s.TotalHits) / float64(s.TotalOps)
}

func (s *CacheStats) AvgDuration() time.Duration {
	if s.TotalOps == 0 {
		return 0
	}
	return s.TotalDuration / time.Duration(s.TotalOps)
}

func (s *CacheStats) Clone() *CacheStats {
	return &CacheStats{
		Hits: s.Hits, Misses: s.Misses, LocalHits: s.LocalHits, RemoteHits: s.RemoteHits,
		TotalOps: s.TotalOps, TotalHits: s.TotalHits, TotalMisses: s.TotalMisses,
		TotalDuration: s.TotalDuration, StartTime: s.StartTime,
	}
}

// CacheEvent represents a cache event.
type CacheEvent struct {
	Type  EventType
	Key   string
	Time  time.Time
	Value []byte
	Error error
}

// EventType is the type of cache event.
type EventType int

const (
	EventGet EventType = iota
	EventSet
	EventDelete
	EventHit
	EventMiss
	EventExpire
)

// CacheListener listens for cache events.
type CacheListener interface {
	OnEvent(event *CacheEvent)
}

// Logger is the cache logger interface.
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Fatal(msg string, fields ...interface{})
}

// Errors
var (
	ErrCacheMiss   = fmt.Errorf("cache miss")
	ErrCacheClosed = fmt.Errorf("cache closed")
)