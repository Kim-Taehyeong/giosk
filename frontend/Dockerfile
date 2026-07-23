# 1. 빌드 단계
FROM node:18-alpine as builder
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
# 같은 오리진(nginx 가 /api 프록시) → 상대경로 /api 로 빌드. CORS 없음.
ARG VITE_API_URL=/api
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

# 2. 실행 단계
FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
# Vite 사용시 dist, CRA 사용시 build
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 443
CMD ["nginx", "-g", "daemon off;"]