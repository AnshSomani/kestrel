const { Pool } = require('pg');
const fs = require('fs');
const path = require('path');
const logger = require('./logger');
const config = require('./config');

const pool = new Pool({
  connectionString: config.databaseUrl,
  max: 15,
  min: 2,
  idleTimeoutMillis: 30000,
  connectionTimeoutMillis: 5000,
});

pool.on('error', (err) => {
  logger.error({ err }, 'Unexpected PostgreSQL pool error');
});

/**
 * Runs all pending .sql migrations from the migrations/ directory in
 * lexicographic order, tracking applied migrations in _migrations table.
 */
async function runMigrations() {
  // Create tracking table first (idempotent)
  await pool.query(`
    CREATE TABLE IF NOT EXISTS _migrations (
      name TEXT PRIMARY KEY,
      applied_at TIMESTAMPTZ DEFAULT NOW()
    )
  `);

  const migrationsDir = path.join(__dirname, '..', 'migrations');
  const files = fs.readdirSync(migrationsDir)
    .filter((f) => f.endsWith('.sql'))
    .sort(); // lexicographic order = 001, 002, ...

  for (const file of files) {
    const { rows } = await pool.query(
      'SELECT name FROM _migrations WHERE name = $1',
      [file],
    );
    if (rows.length > 0) continue; // already applied

    const sql = fs.readFileSync(path.join(migrationsDir, file), 'utf8');
    logger.info(`Running migration: ${file}`);
    await pool.query(sql);
    await pool.query('INSERT INTO _migrations (name) VALUES ($1)', [file]);
    logger.info(`Migration applied: ${file}`);
  }
}

module.exports = { pool, runMigrations };
