/**
 * Admin-only guard. Must be placed AFTER jwtAuth.
 * Returns 403 if the authenticated user does not have the 'admin' role.
 */
function adminOnly(req, res, next) {
  if (!req.user || req.user.role !== 'admin') {
    return res.status(403).json({ error: 'forbidden: requires admin role' });
  }
  return next();
}

module.exports = adminOnly;
