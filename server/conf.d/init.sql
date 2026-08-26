-- =====================================================================
-- scheduler-platform 初始化数据脚本（DML）
-- 表结构由 GORM AutoMigrate 自动同步（server 启动时执行），本脚本只含基础数据。
-- 用途：批量初始化默认账号、节点等，避免页面显示演示/假数据。
-- 用法：
--   SQLite:  sqlite3 scheduler.db < conf.d/init.sql
--   MySQL :  mysql -u root -p scheduler < conf.d/init.sql
-- 说明：脚本幂等（WHERE NOT EXISTS），重复执行不会产生重复数据。
-- =====================================================================

-- 1) 默认管理员（admin / admin123，密码为 bcrypt hash）
INSERT INTO users (username, password, nickname, email, is_admin, status, created_at, updated_at)
SELECT 'admin', '$2a$10$UcTy/yoD5He5FJ62MYtEZu/3MN1LWIe5R7IcV8W2v5.wxf7hRO9jW', '管理员', 'admin@scheduler.local', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM users WHERE username = 'admin');

-- 2) 默认节点（worker 注册表，页面节点监控据此展示真实数据）
INSERT INTO nodes (node_id, name, address, os, version, status, cpu_usage, memory_usage, disk_usage, load_average, running_tasks, max_concurrency, last_heartbeat, created_at, updated_at)
SELECT 'node-001', 'worker-01', '192.168.1.101', 'Linux 5.15', 'v1.0.0', 'online', 0, 0, 0, 0, 0, 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM nodes WHERE node_id = 'node-001');

-- 提示：任务（tasks）与执行记录（executions/task_logs）不提供种子数据，
-- 请通过前端「新建任务 + 手动执行 / 定时调度」产生真实数据。
