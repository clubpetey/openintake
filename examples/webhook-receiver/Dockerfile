# Phase 7-iii — webhook-receiver image for the docker-compose demo.
# Reuses the existing examples/webhook-receiver/server.mjs (Phase 1) which has
# zero npm dependencies (only node:http stdlib). Small alpine base + non-root
# user keeps the container surface minimal.

FROM node:24-alpine

# node:24-alpine ships a `node` user (UID 1000) by default. Use it.
WORKDIR /app

COPY package.json server.mjs ./

EXPOSE 9099

USER node

CMD ["node", "/app/server.mjs"]
