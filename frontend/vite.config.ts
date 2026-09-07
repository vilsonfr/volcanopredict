import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import cesium from "vite-plugin-cesium";

export default defineConfig({
  // O CesiumJS carrega workers, shaders e assets estáticos em runtime. O plugin
  // copia esses arquivos e define CESIUM_BASE_URL; sem ele o globo sobe preto.
  plugins: [react(), cesium()],
  resolve: {
    // Uma unica instancia do engine do Cesium. Com duas copias na arvore, uma
    // cria o contexto WebGL e a outra le seus limites — que continuam zerados,
    // produzindo "maximum texture size (0)" e um globo preto.
    dedupe: ["cesium", "@cesium/engine", "@cesium/widgets"],
  },
  server: {
    // 0.0.0.0 para que o servidor de dev seja alcançável de fora do container.
    host: "0.0.0.0",
    port: 5173,
    strictPort: true,
    watch: {
      // O bind mount do compose não propaga eventos inotify de forma confiável.
      usePolling: true,
    },
  },
  preview: {
    host: "0.0.0.0",
    port: 5173,
    strictPort: true,
  },
});
