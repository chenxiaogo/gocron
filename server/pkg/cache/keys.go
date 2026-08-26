package cache

import "fmt"

// Key prefixes for scheduler-platform cache.
const (
	PrefixTask       = "task"
	PrefixExecution  = "exec"
	PrefixDashboard  = "dash"
	PrefixNode       = "node"
	PrefixUser       = "user"
	PrefixSession    = "session"
)

// KeyTask returns a cache key for a single task.
func KeyTask(taskID int64) string {
	return fmt.Sprintf("%s:%d", PrefixTask, taskID)
}

// KeyTaskList returns a cache key for a task list query.
func KeyTaskList(keyword, ttype string, status, page, pageSize int) string {
	return fmt.Sprintf("%s:list:kw_%s:tp_%s:st_%d:p_%d:ps_%d",
		PrefixTask, keyword, ttype, status, page, pageSize)
}

// KeyExecution returns a cache key for a single execution.
func KeyExecution(execID int64) string {
	return fmt.Sprintf("%s:%d", PrefixExecution, execID)
}

// KeyExecutionList returns a cache key for execution list.
func KeyExecutionList(taskID int64, status string, page, pageSize int) string {
	return fmt.Sprintf("%s:list:tid_%d:st_%s:p_%d:ps_%d",
		PrefixExecution, taskID, status, page, pageSize)
}

// KeyDashboardOverview returns a cache key for dashboard overview.
func KeyDashboardOverview() string {
	return fmt.Sprintf("%s:overview", PrefixDashboard)
}

// KeyDashboardCharts returns a cache key for dashboard charts.
func KeyDashboardCharts() string {
	return fmt.Sprintf("%s:charts", PrefixDashboard)
}

// KeyDashboardTrend returns a cache key for dashboard trend.
func KeyDashboardTrend(rng string) string {
	return fmt.Sprintf("%s:trend:%s", PrefixDashboard, rng)
}

// KeyNodeList returns a cache key for node list.
func KeyNodeList() string {
	return fmt.Sprintf("%s:list", PrefixNode)
}

// KeyNode returns a cache key for a single node.
func KeyNode(nodeID int64) string {
	return fmt.Sprintf("%s:%d", PrefixNode, nodeID)
}

// AllCachePrefixes returns all registered cache key prefixes for Clear operations.
func AllCachePrefixes() []string {
	return []string{
		PrefixTask,
		PrefixExecution,
		PrefixDashboard,
		PrefixNode,
	}
}

// GetClearPattern builds a SCAN pattern for cache clearing.
// Format: {namespace}:{prefix}[:{clientId}]:*
func GetClearPattern(namespace, prefix string, clientId int) string {
	if clientId > 0 {
		return fmt.Sprintf("%s:%s:%d:*", namespace, prefix, clientId)
	}
	return fmt.Sprintf("%s:%s:*", namespace, prefix)
}