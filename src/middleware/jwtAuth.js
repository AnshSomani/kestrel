const jwt = require('jsonwebtoken');
const config = require('../config');

/**
 * JWT Bearer token authentication middleware.
 * Validates the Authorization: Bearer <token> header using HS256.
 * Injects req.user (full claims) and req.userId on success.
 */
function jwtAuth(req, res, next) {
  const authHeader = req.headers['authorization'];
  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return res.status(401).json({ error: 'missing or invalid authorization header' });
  }

  const token = authHeader.slice(7);
  try {
    const claims = jwt.verify(token, config.jwtSecret, { algorithms: ['HS256'] });
    req.user = claims;
    req.userId = claims.user_id;
    return next();
  } catch (err) {
    return res.status(401).json({ error: 'invalid or expired token' });
  }
}

module.exports = jwtAuth;
