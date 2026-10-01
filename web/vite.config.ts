import { fileURLToPath, URL } from 'node:url';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';

const target = process.env.ADMIN_API_TARGET || 'http://127.0.0.1:8080';
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { proxy: { '/api': { target, changeOrigin: true }, '/avatars': { target, changeOrigin: true }, '/docs': { target, changeOrigin: true } } },
  test: { environment: 'node', clearMocks: true },
});
