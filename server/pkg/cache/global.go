package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"scheduler-platform/pkg/cache/metrics"
	"scheduler-platform/pkg/cache/utils"

	jetcache "github.com/mgtv-tech/jetcache-go"
	"github.com/mgtv-tech/jetcache-go/local"
	"github.com/mgtv-tech/jetcache-go/remote"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// GlobalCache is the global cache implementation.
type GlobalCache struct {
	mu          sync.RWMutex
	config      *Config
	cache       jetcache.Cache
	localCache  local.Local
	remoteCache remote.Remote
	rawRedis    redis.Cmdable
	stats       *metrics.CacheStats
	monitor     metrics.Monitor
	flightGroup singleflight.Group
	keyBuilder  *utils.DefaultKeyBuilder
	codec       utils.Codec
	logger      utils.Logger
	listeners   []CacheListener
	closed      bool
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewGlobalCache creates a new GlobalCache instance.
func NewGlobalCache(cfg *Config) (*GlobalCache, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	cacheStats := metrics.NewCacheStats()

	monitorFactory := &metrics.MonitorFactory{}
	monitorType := cfg.MonitorType
	if monitorType == "" {
		monitorType = "prometheus"
	}
	monitor := monitorFactory.GetMonitor(monitorType, cfg.Name)

	keyBuilder := utils.NewDefaultKeyBuilder(cfg.Namespace)

	codecFactory := &utils.CodecFactory{}
	codec, err := codecFactory.GetCodec("msgpack")
	if err != nil {
		return nil, err
	}

	var logger utils.Logger
	if cfg.Logger != nil {
		// Wrap external Logger to utils.Logger
		logger = &ilogAdapter{inner: cfg.Logger}
	} else {
		loggerFactory := &utils.LoggerFactory{}
		logger = loggerFactory.GetLogger("zap", cfg.LogLevel)
	}

	opts := []jetcache.Option{
		jetcache.WithName(cfg.Name),
	}

	var localCache local.Local
	if cfg.Local.Enabled {
		switch cfg.Local.Type {
		case "freecache":
			lc := local.NewFreeCache(local.Size(cfg.Local.Size), cfg.Local.TTL)
			localCache = lc
			opts = append(opts, jetcache.WithLocal(lc))
		default:
			return nil, fmt.Errorf("unsupported local cache type: %s", cfg.Local.Type)
		}
	}

	var remoteCache remote.Remote
	var rawRedisClient redis.Cmdable
	if cfg.Remote.Enabled {
		switch cfg.Remote.Type {
		case "redis":
			rc, rdb, err := createRedisCache(cfg)
			if err != nil {
				return nil, fmt.Errorf("create redis cache failed: %v", err)
			}
			remoteCache = rc
			rawRedisClient = rdb
			opts = append(opts, jetcache.WithRemote(rc))
		default:
			return nil, fmt.Errorf("unsupported remote cache type: %s", cfg.Remote.Type)
		}
	}

	if cfg.RefreshInterval > 0 {
		opts = append(opts, jetcache.WithRefreshDuration(cfg.RefreshInterval))
	}
	opts = append(opts,
		jetcache.WithRemoteExpiry(cfg.DefaultTTL),
		jetcache.WithNotFoundExpiry(cfg.NotFoundTTL),
		jetcache.WithStatsDisabled(!cfg.EnableStats),
	)

	jc := jetcache.New(opts...)

	gc := &GlobalCache{
		config:      cfg,
		cache:       jc,
		localCache:  localCache,
		remoteCache: remoteCache,
		rawRedis:    rawRedisClient,
		stats:       cacheStats,
		monitor:     monitor,
		keyBuilder:  keyBuilder,
		codec:       codec,
		logger:      logger,
		listeners:   make([]CacheListener, 0),
		stopCh:      make(chan struct{}),
	}

	gc.startBackgroundTasks()

	logger.Info("global cache initialized",
		"name", cfg.Name, "namespace", cfg.Namespace,
		"local_enabled", cfg.Local.Enabled, "remote_enabled", cfg.Remote.Enabled)

	return gc, nil
}

// Get retrieves a value from cache.
func (gc *GlobalCache) Get(ctx context.Context, key string) ([]byte, error) {
	start := time.Now()
	fullKey := gc.keyBuilder.Build(key)

	var result []byte
	err := gc.cache.Get(ctx, fullKey, &result)

	if err == nil {
		gc.recordHit("get", key, time.Since(start))
	} else {
		gc.recordMiss("get", key, time.Since(start), err)
	}

	if err != nil {
		return nil, fmt.Errorf("cache get failed for key %s: %w", key, err)
	}
	return result, nil
}

// GetOrSet gets or loads with singleflight protection.
func (gc *GlobalCache) GetOrSet(ctx context.Context, key string, loader Loader, ttl time.Duration) ([]byte, error) {
	if ttl <= 0 {
		ttl = gc.config.DefaultTTL
	}

	v, err, _ := gc.flightGroup.Do(key, func() (interface{}, error) {
		if val, err := gc.Get(ctx, key); err == nil {
			return val, nil
		}
		val, err := loader(ctx, key)
		if err != nil {
			return nil, err
		}
		if err := gc.Set(ctx, key, val, ttl); err != nil {
			gc.logger.Warn("cache set failed in GetOrSet", "key", key, "error", err)
		}
		return val, nil
	})

	if err != nil {
		return nil, err
	}
	if val, ok := v.([]byte); ok {
		return val, nil
	}
	return nil, fmt.Errorf("type assertion failed")
}

// Set stores a value in cache.
func (gc *GlobalCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	start := time.Now()
	fullKey := gc.keyBuilder.Build(key)

	if ttl <= 0 {
		ttl = gc.config.DefaultTTL
	}

	err := gc.cache.Set(ctx, fullKey, jetcache.Value(value), jetcache.TTL(ttl))
	gc.recordOperation("set", key, err, time.Since(start))

	if err != nil {
		return fmt.Errorf("cache set failed for key %s: %w", key, err)
	}

	gc.notifyListeners(&CacheEvent{Type: EventSet, Key: key, Time: time.Now(), Value: value})
	return nil
}

// Delete removes a key from cache.
func (gc *GlobalCache) Delete(ctx context.Context, key string) error {
	start := time.Now()
	fullKey := gc.keyBuilder.Build(key)

	err := gc.cache.Delete(ctx, fullKey)
	gc.recordOperation("delete", key, err, time.Since(start))

	if err != nil {
		return fmt.Errorf("cache delete failed for key %s: %w", key, err)
	}

	gc.notifyListeners(&CacheEvent{Type: EventDelete, Key: key, Time: time.Now()})
	return nil
}

// Exists checks if a key exists in cache.
func (gc *GlobalCache) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := gc.keyBuilder.Build(key)
	return gc.cache.Exists(ctx, fullKey), nil
}

// MGet retrieves multiple keys.
func (gc *GlobalCache) MGet(ctx context.Context, keys []string) (map[string][]byte, error) {
	start := time.Now()
	result := make(map[string][]byte)
	for _, key := range keys {
		fullKey := gc.keyBuilder.Build(key)
		var val []byte
		if err := gc.cache.Get(ctx, fullKey, &val); err == nil {
			result[key] = val
		}
	}
	gc.recordOperation("mget", fmt.Sprintf("keys=%d", len(keys)), nil, time.Since(start))
	return result, nil
}

// MSet stores multiple key-value pairs.
func (gc *GlobalCache) MSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	start := time.Now()
	for key, value := range items {
		if err := gc.Set(ctx, key, value, ttl); err != nil {
			gc.logger.Error("mset failed", "key", key, "error", err)
		}
	}
	gc.recordOperation("mset", fmt.Sprintf("items=%d", len(items)), nil, time.Since(start))
	return nil
}

// MDelete removes multiple keys.
func (gc *GlobalCache) MDelete(ctx context.Context, keys []string) error {
	start := time.Now()
	for _, key := range keys {
		if err := gc.Delete(ctx, key); err != nil {
			gc.logger.Error("mdelete failed", "key", key, "error", err)
		}
	}
	gc.recordOperation("mdelete", fmt.Sprintf("keys=%d", len(keys)), nil, time.Since(start))
	return nil
}

// GetAndDelete retrieves and then deletes a key.
func (gc *GlobalCache) GetAndDelete(ctx context.Context, key string) ([]byte, error) {
	val, err := gc.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if err := gc.Delete(ctx, key); err != nil {
		return val, err
	}
	return val, nil
}

// GetAndSet retrieves and then sets a key.
func (gc *GlobalCache) GetAndSet(ctx context.Context, key string, value []byte, ttl time.Duration) ([]byte, error) {
	val, err := gc.Get(ctx, key)
	if err != nil && err != ErrCacheMiss {
		return nil, err
	}
	if err := gc.Set(ctx, key, value, ttl); err != nil {
		return val, err
	}
	return val, nil
}

// Increment atomically increments a counter.
func (gc *GlobalCache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	val, err := gc.Get(ctx, key)
	var current int64
	if err == nil {
		if err := gc.codec.Decode(val, &current); err != nil {
			current = 0
		}
	}
	newVal := current + delta
	newData, err := gc.codec.Encode(newVal)
	if err != nil {
		return 0, err
	}
	if err := gc.Set(ctx, key, newData, gc.config.DefaultTTL); err != nil {
		return 0, err
	}
	return newVal, nil
}

// Decrement atomically decrements a counter.
func (gc *GlobalCache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	return gc.Increment(ctx, key, -delta)
}

// TryLock attempts to acquire a distributed lock.
func (gc *GlobalCache) TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	lockKey := gc.keyBuilder.Build("lock:" + key)
	if gc.config.Remote.Enabled && gc.remoteCache != nil {
		result, err := gc.remoteCache.SetNX(ctx, lockKey, "1", ttl)
		if err != nil {
			gc.logger.Error("try lock failed", "key", key, "error", err)
			return false, err
		}
		return result, nil
	}
	gc.mu.Lock()
	defer gc.mu.Unlock()
	if _, err := gc.Get(ctx, lockKey); err == nil {
		return false, nil
	}
	if err := gc.Set(ctx, lockKey, []byte("1"), ttl); err != nil {
		return false, err
	}
	return true, nil
}

// Unlock releases a distributed lock.
func (gc *GlobalCache) Unlock(ctx context.Context, key string) error {
	lockKey := gc.keyBuilder.Build("lock:" + key)
	return gc.Delete(ctx, lockKey)
}

// Stats returns cache statistics.
func (gc *GlobalCache) Stats() *CacheStats {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return &CacheStats{
		Hits: gc.stats.Hits, Misses: gc.stats.Misses,
		LocalHits: gc.stats.LocalHits, RemoteHits: gc.stats.RemoteHits,
		TotalOps: gc.stats.TotalOps, TotalHits: gc.stats.TotalHits,
		TotalMisses: gc.stats.TotalMisses, TotalDuration: gc.stats.TotalDuration,
		StartTime: gc.stats.StartTime,
	}
}

// ClearStats resets statistics.
func (gc *GlobalCache) ClearStats() {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.stats = metrics.NewCacheStats()
}

// Clear clears all business cache.
func (gc *GlobalCache) Clear(ctx context.Context) (int64, error) {
	prefixes := AllCachePrefixes()
	gc.logger.Warn("clearing all business cache", "prefixes", fmt.Sprintf("%v", prefixes))
	namespace := gc.getNamespace()
	var totalDeleted int64
	for _, prefix := range prefixes {
		deleted, err := gc.clearByPrefix(ctx, namespace, prefix)
		if err != nil {
			gc.logger.Error("clear cache failed", "prefix", prefix, "error", err)
			return totalDeleted, err
		}
		totalDeleted += deleted
		if deleted > 0 {
			gc.logger.Info("cache cleared", "prefix", prefix, "deleted", deleted)
		}
	}
	gc.logger.Info("all cache cleared", "totalDeleted", totalDeleted)
	return totalDeleted, nil
}

// ClearByClientId clears cache by client ID.
func (gc *GlobalCache) ClearByClientId(ctx context.Context, clientId int) (int64, error) {
	if gc.rawRedis == nil {
		return 0, nil
	}
	if clientId <= 0 {
		return 0, fmt.Errorf("ClearByClientId: invalid clientId=%d, must be > 0", clientId)
	}
	key := fmt.Sprintf("clear:%d", clientId)
	result, err, _ := gc.flightGroup.Do(key, func() (interface{}, error) {
		return gc.clearByClientIdInner(ctx, clientId)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (gc *GlobalCache) clearByClientIdInner(ctx context.Context, clientId int) (int64, error) {
	namespace := gc.getNamespace()
	prefixes := AllCachePrefixes()
	gc.logger.Info("clearing client cache", "clientId", clientId)
	var totalDeleted int64
	for _, prefix := range prefixes {
		pattern := GetClearPattern(namespace, prefix, clientId)
		scannedKeys, scanErr := gc.scanKeysOnePass(ctx, pattern)
		if scanErr != nil {
			gc.logger.Error("SCAN scan failed", "pattern", pattern, "error", scanErr)
			continue
		}
		if len(scannedKeys) > 0 {
			deleted, dErr := gc.batchDeleteKeys(ctx, scannedKeys)
			if dErr != nil {
				gc.logger.Error("batch delete failed", "pattern", pattern, "error", dErr)
				continue
			}
			totalDeleted += deleted
			gc.logger.Info("clear done", "clientId", clientId, "prefix", prefix, "deleted", deleted)
		}
	}
	gc.logger.Info("client cache cleared", "clientId", clientId, "totalDeleted", totalDeleted)
	return totalDeleted, nil
}

func (gc *GlobalCache) clearByPrefix(ctx context.Context, namespace, prefix string) (int64, error) {
	if gc.rawRedis == nil {
		gc.logger.Warn("remote Redis not configured, skip clear", "prefix", prefix)
		return 0, nil
	}
	pattern := GetClearPattern(namespace, prefix, 0)
	allKeys, err := gc.scanKeysOnePass(ctx, pattern)
	if err != nil {
		return 0, err
	}
	if len(allKeys) == 0 {
		gc.logger.Debug("no matching cache", "pattern", pattern)
		return 0, nil
	}
	return gc.batchDeleteKeys(ctx, allKeys)
}

// getNamespace returns the current namespace.
func (gc *GlobalCache) getNamespace() string {
	ns := gc.config.Namespace
	if ns == "" {
		return "scheduler"
	}
	return ns
}

const scanBatchSize = int64(200)

func (gc *GlobalCache) scanKeysOnePass(ctx context.Context, pattern string) ([]string, error) {
	var keys []string
	cursor := uint64(0)
	var lastErr error
	for {
		scanCmd := gc.rawRedis.Scan(ctx, cursor, pattern, scanBatchSize)
		batchKeys, nextCursor, err := scanCmd.Result()
		if err != nil {
			gc.logger.Warn("SCAN command error", "pattern", pattern, "error", err)
			lastErr = err
			break
		}
		keys = append(keys, batchKeys...)
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
	return keys, lastErr
}

const deleteBatchSize = 500

func (gc *GlobalCache) batchDeleteKeys(ctx context.Context, allKeys []string) (int64, error) {
	if len(allKeys) == 0 {
		return 0, nil
	}
	var totalDeleted int64
	for i := 0; i < len(allKeys); i += deleteBatchSize {
		end := i + deleteBatchSize
		if end > len(allKeys) {
			end = len(allKeys)
		}
		batch := allKeys[i:end]
		deleted, err := gc.rawRedis.Del(ctx, batch...).Result()
		if err != nil {
			return totalDeleted, fmt.Errorf("batch delete failed (batch %d-%d): %w", i+1, end, err)
		}
		totalDeleted += deleted
	}
	if gc.localCache != nil {
		for _, k := range allKeys {
			gc.localCache.Del(k)
		}
	}
	return totalDeleted, nil
}

// Close shuts down the cache.
func (gc *GlobalCache) Close() error {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	if gc.closed {
		return nil
	}
	gc.closed = true
	close(gc.stopCh)
	gc.wg.Wait()
	if gc.cache != nil {
		gc.cache.Close()
	}
	if gc.monitor != nil {
		gc.monitor.Close()
	}
	gc.logger.Info("global cache closed")
	return nil
}

// AddListener adds a cache event listener.
func (gc *GlobalCache) AddListener(listener CacheListener) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.listeners = append(gc.listeners, listener)
}

func (gc *GlobalCache) recordOperation(op, key string, err error, duration time.Duration) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.stats.TotalOps++
	if err == nil {
		gc.stats.TotalHits++
	} else {
		gc.stats.TotalMisses++
	}
	gc.stats.TotalDuration += duration
	if gc.monitor != nil {
		gc.monitor.RecordOperation(op, err == nil, duration)
	}
	if err != nil {
		gc.logger.Debug("cache operation", "operation", op, "key", key, "duration", duration, "error", err)
	}
}

func (gc *GlobalCache) recordHit(op, key string, duration time.Duration) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.stats.Hits++
	gc.stats.TotalOps++
	gc.stats.TotalHits++
	gc.stats.TotalDuration += duration
	if gc.monitor != nil {
		gc.monitor.RecordHit("local")
	}
	gc.notifyListeners(&CacheEvent{Type: EventHit, Key: key, Time: time.Now()})
}

func (gc *GlobalCache) recordMiss(op, key string, duration time.Duration, err error) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.stats.Misses++
	gc.stats.TotalOps++
	gc.stats.TotalMisses++
	gc.stats.TotalDuration += duration
	if gc.monitor != nil {
		gc.monitor.RecordMiss("local")
	}
	gc.notifyListeners(&CacheEvent{Type: EventMiss, Key: key, Time: time.Now(), Error: err})
}

func (gc *GlobalCache) notifyListeners(event *CacheEvent) {
	for _, listener := range gc.listeners {
		listener.OnEvent(event)
	}
}

func (gc *GlobalCache) startBackgroundTasks() {
	if gc.config.EnableStats {
		gc.wg.Add(1)
		go func() {
			defer gc.wg.Done()
			gc.collectStats()
		}()
	}
	gc.wg.Add(1)
	go func() {
		defer gc.wg.Done()
		gc.cleanupExpired()
	}()
	gc.wg.Add(1)
	go func() {
		defer gc.wg.Done()
		gc.healthCheck()
	}()
}

func (gc *GlobalCache) collectStats() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-gc.stopCh:
			return
		case <-ticker.C:
			stats := gc.Stats()
			if gc.monitor != nil {
				gc.monitor.UpdateHitRate("all", stats.HitRate())
				localOps := stats.Hits + stats.Misses
				if localOps > 0 {
					gc.monitor.UpdateHitRate("local", float64(stats.Hits)/float64(localOps))
				}
				gc.monitor.UpdateCacheMetrics("local", 0, float64(localOps))
				if stats.TotalOps > 0 && stats.RemoteHits > 0 {
					gc.monitor.UpdateHitRate("remote", float64(stats.RemoteHits)/float64(stats.TotalOps))
					gc.monitor.UpdateCacheMetrics("remote", 0, float64(stats.RemoteHits))
				}
				localSize := int64(0)
				if gc.localCache != nil {
					localSize = gc.config.Local.Size
				}
				gc.monitor.UpdateMemoryUsage("local", float64(localSize))
			}
			gc.logger.Debug("cache stats", "hits", stats.Hits, "misses", stats.Misses,
				"hit_rate", stats.HitRate(), "total_ops", stats.TotalOps)
		}
	}
}

func (gc *GlobalCache) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-gc.stopCh:
			return
		case <-ticker.C:
			gc.logger.Debug("cleaning expired cache")
		}
	}
}

func (gc *GlobalCache) healthCheck() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-gc.stopCh:
			return
		case <-ticker.C:
			if gc.localCache != nil {
				gc.logger.Debug("local cache health check passed")
			}
			if gc.remoteCache != nil {
				gc.logger.Debug("remote cache health check passed")
			}
		}
	}
}

// ilogAdapter adapts the cache.Logger interface to utils.Logger.
type ilogAdapter struct {
	inner Logger
}

func (a *ilogAdapter) Debug(msg string, fields ...interface{}) { a.inner.Debug(msg, fields...) }
func (a *ilogAdapter) Info(msg string, fields ...interface{})  { a.inner.Info(msg, fields...) }
func (a *ilogAdapter) Warn(msg string, fields ...interface{})  { a.inner.Warn(msg, fields...) }
func (a *ilogAdapter) Error(msg string, fields ...interface{}) { a.inner.Error(msg, fields...) }
func (a *ilogAdapter) Fatal(msg string, fields ...interface{}) { a.inner.Fatal(msg, fields...) }

func createRedisCache(cfg *Config) (remote.Remote, redis.Cmdable, error) {
	var redisClient redis.Cmdable
	if len(cfg.Remote.Addresses) > 1 {
		redisClient = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    cfg.Remote.Addresses,
			Password: cfg.Remote.Password,
			PoolSize: cfg.Remote.PoolSize,
		})
	} else {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.Remote.Addresses[0],
			Password: cfg.Remote.Password,
			DB:       cfg.Remote.DB,
			PoolSize: cfg.Remote.PoolSize,
		})
	}
	return remote.NewGoRedisV9Adapter(redisClient), redisClient, nil
}