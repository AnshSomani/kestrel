const apiKeyAuth = require('./apiKeyAuth');
const jwtAuth = require('./jwtAuth');

/**
 * Accepts either X-API-Key header OR Authorization: Bearer JWT.
 * Tries API key first; falls back to JWT if the header is absent.
 * Returns 401 only if both methods fail.
 * Mirrors Go's MultiAuthMiddleware.
 */
function multiAuth(req, res, next) {
  if (req.headers['x-api-key']) {
    return apiKeyAuth(req, res, next);
  }
  return jwtAuth(req, res, next);
}

module.exports = multiAuth;
