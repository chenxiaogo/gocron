package cache

import (
	"context"
	"time"

	"scheduler-platform/pkg/cache/utils"
)

// TypedCacheImpl is a generic typed cache wrapper.
type TypedCacheImpl[T any] struct {
	cache  Cache
	prefix string
	codec  utils.Codec
}

// NewTypedCache creates a typed cache.
func NewTypedCache[T any](cache Cache, prefix string) *TypedCacheImpl[T] {
	return &TypedCacheImpl[T]{
		cache:  cache,
		prefix: prefix,
		codec:  utils.NewJSONCodec(),
	}
}

// Get retrieves and decodes a value.
func (tc *TypedCacheImpl[T]) Get(ctx context.Context, key string) (*T, error) {
	fullKey := tc.prefix + ":" + key
	data, err := tc.cache.Get(ctx, fullKey)
	if err != nil {
		return nil, err
	}
	var obj T
	if err := tc.codec.Decode(data, &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

// Set encodes and stores a value.
func (tc *TypedCacheImpl[T]) Set(ctx context.Context, key string, value *T, ttl time.Duration) error {
	fullKey := tc.prefix + ":" + key
	data, err := tc.codec.Encode(value)
	if err != nil {
		return err
	}
	return tc.cache.Set(ctx, fullKey, data, ttl)
}

// Delete removes a key.
func (tc *TypedCacheImpl[T]) Delete(ctx context.Context, key string) error {
	fullKey := tc.prefix + ":" + key
	return tc.cache.Delete(ctx, fullKey)
}

// GetOrSet gets or loads with singleflight protection.
func (tc *TypedCacheImpl[T]) GetOrSet(ctx context.Context, key string, loader TypedLoader[T], ttl time.Duration) (*T, error) {
	fullKey := tc.prefix + ":" + key
	data, err := tc.cache.GetOrSet(ctx, fullKey, func(ctx context.Context, key string) ([]byte, error) {
		obj, loadErr := loader(ctx, key)
		if loadErr != nil {
			return nil, loadErr
		}
		return tc.codec.Encode(obj)
	}, ttl)
	if err != nil {
		return nil, err
	}
	var result T
	if err := tc.codec.Decode(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}