# ─── Stage 1: install production dependencies ────────────────────────────────
FROM node:22-alpine AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci --omit=dev

# ─── Stage 2: runtime image ──────────────────────────────────────────────────
FROM node:22-alpine
WORKDIR /app

# Copy installed modules from deps stage
COPY --from=deps /app/node_modules ./node_modules

# Copy application source and migrations
COPY package*.json ./
COPY src/ ./src/
COPY migrations/ ./migrations/

EXPOSE 8080
CMD ["node", "src/index.js"]
