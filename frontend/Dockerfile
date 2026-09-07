# Dependências resolvidas a partir do lockfile, em camada própria para
# aproveitar o cache: só reinstala quando package.json/package-lock.json mudam.
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

# Verifica que o projeto compila. Não produz a imagem final na V0.1 — serve
# como portão de build, para que erro de TypeScript quebre `docker compose build`
# em vez de aparecer só em produção.
FROM deps AS build
WORKDIR /app
COPY . .
RUN npm run build

# Imagem executada pelo compose: servidor de desenvolvimento do Vite.
# Herda de `build` (e não de `deps`) de propósito: assim o estágio de build é
# obrigatoriamente executado, e erro de TypeScript quebra `docker compose build`.
# Quando houver deploy (V0.3+), entra um estágio novo servindo os estáticos
# de `build` atrás de um servidor HTTP.
FROM build AS dev
WORKDIR /app
EXPOSE 5173
CMD ["npm", "run", "dev"]
