package xlog

import "sync/atomic"

var (
	globalFields atomic.Value
)

type fieldsKey struct{}
