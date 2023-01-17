import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react({
    jsxImportSource: '@emotion/react',
    babel: {
      plugins: ['@emotion/babel-plugin'],
    },
  })],
  resolve: {
    alias: {
      react: 'https://esm.sh/react@18.2.0',
      'react-dom': 'https://esm.sh/react-dom@18.2.0'
    },
  },
  build: {
    minify: false,
    rollupOptions: {
      output: {
        assetFileNames: 'assets/[name].[ext]',
        entryFileNames: 'assets/main.js',
        chunkFileNames: 'assets/[name].[hash].[ext]',
      }
    }
  }
})
