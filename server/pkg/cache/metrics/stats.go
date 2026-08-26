package metrics

import "time"

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

func NewCacheStats() *CacheStats {
	return &CacheStats{StartTime: time.Now()}
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
		Hits:          s.Hits,
		Misses:        s.Misses,
		LocalHits:     s.LocalHits,
		RemoteHits:    s.RemoteHits,
		TotalOps:      s.TotalOps,
		TotalHits:     s.TotalHits,
		TotalMisses:   s.TotalMisses,
		TotalDuration: s.TotalDuration,
		StartTime:     s.StartTime,
	}
}