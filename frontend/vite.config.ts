import vue from "@vitejs/plugin-vue";
import { fileURLToPath, URL } from "node:url";
import { defineConfig, type Plugin } from "vite";

/** 与 backend/web/managed_file_image.go 同规则：站内 Markdown 图址在 dev 下也能直接出图 */
function managedFileImageRedirect(): Plugin {
  const managedFileIDPath = /^\/files\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\/?$/i;
  return {
    name: "openshare-managed-file-image-redirect",
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const requestPath = (req.url ?? "").split("?")[0];
        const match = managedFileIDPath.exec(requestPath);
        if (!match || (req.headers.accept ?? "").includes("text/html")) {
          return next();
        }
        res.writeHead(307, { Location: `/api/public/files/${match[1]}/download` });
        res.end();
      });
    },
  };
}

export default defineConfig({
  plugins: [vue(), managedFileImageRedirect()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    host: "0.0.0.0",
    allowedHosts: true,
    port: 5173,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
      "/dl": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
    },
  },
});
