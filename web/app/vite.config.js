var _a;
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
export default defineConfig({
    plugins: [react()],
    server: {
        port: 5173,
        proxy: {
            // Proxy API calls to the NestJS server during development.
            '/api': {
                target: (_a = process.env.VITE_API_TARGET) !== null && _a !== void 0 ? _a : 'http://localhost:3001',
                changeOrigin: true,
            },
        },
    },
});
