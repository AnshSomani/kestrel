-- 006_remove_stats_triggers.sql
-- Removes the synchronous triggers that update system_stats.
-- Stats are now updated asynchronously in memory by the Go backend to prevent row contention.

DROP TRIGGER IF EXISTS trg_events_stats_trigger ON events;
DROP TRIGGER IF EXISTS trg_delivery_jobs_stats_trigger ON delivery_jobs;

DROP FUNCTION IF EXISTS trg_events_stats();
DROP FUNCTION IF EXISTS trg_delivery_jobs_stats();
