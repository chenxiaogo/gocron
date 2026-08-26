package utils

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// KeyBuilder builds cache keys with namespace prefix.
type KeyBuilder interface {
	Build(key string) string
	WithNamespace(namespace string) KeyBuilder
}

// DefaultKeyBuilder is the default key builder.
type DefaultKeyBuilder struct {
	namespace string
	separator string
}

func NewDefaultKeyBuilder(namespace string) *DefaultKeyBuilder {
	return &DefaultKeyBuilder{namespace: namespace, separator: ":"}
}

func (b *DefaultKeyBuilder) Build(key string) string {
	if b.namespace == "" {
		return key
	}
	return fmt.Sprintf("%s%s%s", b.namespace, b.separator, key)
}

func (b *DefaultKeyBuilder) WithNamespace(namespace string) KeyBuilder {
	return &DefaultKeyBuilder{namespace: namespace, separator: b.separator}
}

// HashKeyBuilder builds hashed keys.
type HashKeyBuilder struct {
	namespace string
	separator string
}

func NewHashKeyBuilder(namespace string) *HashKeyBuilder {
	return &HashKeyBuilder{namespace: namespace, separator: ":"}
}

func (b *HashKeyBuilder) Build(key string) string {
	h := fnv32(key)
	if b.namespace == "" {
		return fmt.Sprintf("%d", h)
	}
	return fmt.Sprintf("%s%s%d", b.namespace, b.separator, h)
}

func (b *HashKeyBuilder) WithNamespace(namespace string) KeyBuilder {
	return &HashKeyBuilder{namespace: namespace, separator: b.separator}
}

// CompositeKeyBuilder builds composite keys with optional hash prefix.
type CompositeKeyBuilder struct {
	namespace  string
	separator  string
	hashPrefix bool
}

func NewCompositeKeyBuilder(namespace string, hashPrefix bool) *CompositeKeyBuilder {
	return &CompositeKeyBuilder{namespace: namespace, separator: ":", hashPrefix: hashPrefix}
}

func (b *CompositeKeyBuilder) Build(key string) string {
	if b.hashPrefix {
		prefix := fnv32(key) % 1000
		if b.namespace == "" {
			return fmt.Sprintf("%d:%s", prefix, key)
		}
		return fmt.Sprintf("%s%s%d:%s", b.namespace, b.separator, prefix, key)
	}
	if b.namespace == "" {
		return key
	}
	return fmt.Sprintf("%s%s%s", b.namespace, b.separator, key)
}

func (b *CompositeKeyBuilder) WithNamespace(namespace string) KeyBuilder {
	return &CompositeKeyBuilder{namespace: namespace, separator: b.separator, hashPrefix: b.hashPrefix}
}

func fnv32(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}

// KeyPattern matches keys against a pattern.
type KeyPattern struct {
	pattern string
	parts   []string
}

func NewKeyPattern(pattern string) *KeyPattern {
	return &KeyPattern{pattern: pattern, parts: strings.Split(pattern, ":")}
}

func (k *KeyPattern) Match(key string) bool {
	keyParts := strings.Split(key, ":")
	if len(keyParts) != len(k.parts) {
		return false
	}
	for i, part := range k.parts {
		if part != "*" && part != keyParts[i] {
			return false
		}
	}
	return true
}

func (k *KeyPattern) ExtractParams(key string) map[string]string {
	keyParts := strings.Split(key, ":")
	if len(keyParts) != len(k.parts) {
		return nil
	}
	params := make(map[string]string)
	for i, part := range k.parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			paramName := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			params[paramName] = keyParts[i]
		}
	}
	return params
}