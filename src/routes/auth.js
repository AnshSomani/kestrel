const express = require('express');
const bcrypt = require('bcryptjs');
const jwt = require('jsonwebtoken');
const { pool } = require('../db');
const config = require('../config');
const logger = require('../logger');

const router = express.Router();

function generateTokens(userId, email, role) {
  const payload = { user_id: userId, email, role };
  const accessToken = jwt.sign(payload, config.jwtSecret, {
    algorithm: 'HS256',
    expiresIn: '15m',
    subject: userId,
  });
  const refreshToken = jwt.sign(payload, config.jwtSecret, {
    algorithm: 'HS256',
    expiresIn: '7d',
    subject: userId,
  });
  return { accessToken, refreshToken };
}

function setRefreshCookie(res, token) {
  res.cookie('refresh_token', token, {
    httpOnly: true,
    secure: false, // set true behind HTTPS in production
    maxAge: 7 * 24 * 3600 * 1000,
    path: '/',
    sameSite: 'lax',
  });
}

// POST /api/auth/signup
router.post('/signup', async (req, res) => {
  const { email, password } = req.body;
  if (!email || !password) {
    return res.status(400).json({ error: 'email and password are required' });
  }
  try {
    const hash = await bcrypt.hash(password, 12);
    const { rows } = await pool.query(
      `INSERT INTO users (email, password_hash, role)
       VALUES ($1, $2, 'customer')
       RETURNING id, role`,
      [email, hash],
    );
    const { id: userId, role } = rows[0];
    const { accessToken, refreshToken } = generateTokens(userId, email, role);
    setRefreshCookie(res, refreshToken);
    return res.status(201).json({ access_token: accessToken });
  } catch (err) {
    if (err.code === '23505') { // unique_violation
      return res.status(409).json({ error: 'email already in use' });
    }
    logger.error({ err }, 'Signup error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// POST /api/auth/login
router.post('/login', async (req, res) => {
  const { email, password } = req.body;
  if (!email || !password) {
    return res.status(400).json({ error: 'email and password are required' });
  }
  try {
    const { rows } = await pool.query(
      'SELECT id, password_hash, role FROM users WHERE email = $1',
      [email],
    );
    if (rows.length === 0) {
      return res.status(401).json({ error: 'invalid credentials' });
    }
    const { id: userId, password_hash: hash, role } = rows[0];
    const valid = await bcrypt.compare(password, hash);
    if (!valid) {
      return res.status(401).json({ error: 'invalid credentials' });
    }
    const { accessToken, refreshToken } = generateTokens(userId, email, role);
    setRefreshCookie(res, refreshToken);
    return res.json({ access_token: accessToken });
  } catch (err) {
    logger.error({ err }, 'Login error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// POST /api/auth/refresh
router.post('/refresh', (req, res) => {
  const token = req.cookies?.refresh_token;
  if (!token) {
    return res.status(401).json({ error: 'missing refresh token' });
  }
  try {
    const claims = jwt.verify(token, config.jwtSecret, { algorithms: ['HS256'] });
    const { accessToken, refreshToken } = generateTokens(
      claims.user_id, claims.email, claims.role,
    );
    setRefreshCookie(res, refreshToken);
    return res.json({ access_token: accessToken });
  } catch {
    return res.status(401).json({ error: 'invalid refresh token' });
  }
});

// POST /api/auth/logout
router.post('/logout', (_req, res) => {
  res.cookie('refresh_token', '', {
    httpOnly: true,
    maxAge: 0,
    path: '/',
  });
  return res.json({ status: 'logged out' });
});

module.exports = router;
