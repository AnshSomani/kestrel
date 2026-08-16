const { pool } = require('../db');

/**
 * API Key authentication middleware.
 * Reads X-API-Key header, looks up the key in api_keys table,
 * and injects req.userId on success.
 * Returns 401 if the key is missing or invalid.
 */
async function apiKeyAuth(req, res, next) {
  const key = req.headers['x-api-key'];
  if (!key) {
    return res.status(401).json({ error: 'missing API key' });
  }

  try {
    const { rows } = await pool.query(
      'SELECT user_id FROM api_keys WHERE key = $1',
      [key],
    );
    if (rows.length === 0) {
      return res.status(401).json({ error: 'invalid API key' });
    }
    req.userId = rows[0].user_id;
    return next();
  } catch (err) {
    return res.status(500).json({ error: 'internal server error' });
  }
}

module.exports = apiKeyAuth;
