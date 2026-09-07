import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
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
