FROM node:18-alpine

WORKDIR /app

COPY upstream_forwarder.js .

ENV NODE_ENV=production

ENV PORT=8787
ENV HOST=0.0.0.0

EXPOSE 8787

CMD ["node", "upstream_forwarder.js"]