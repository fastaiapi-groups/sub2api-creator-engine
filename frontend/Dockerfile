FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json vite.config.ts index.html ./
RUN npm ci
COPY src ./src
RUN npm run build
FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
