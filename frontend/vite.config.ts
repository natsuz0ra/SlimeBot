import path from 'path'
import { existsSync, readFileSync } from 'fs'
import { homedir } from 'os'
import { dirname, join } from 'path'
import { fileURLToPath } from 'url'
import { defineConfig, searchForWorkspaceRoot } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

const packageJson = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf-8')) as { version?: string }
const __dirname = dirname(fileURLToPath(import.meta.url))

const DEFAULT_SERVER_PORT = '6247'
const DEFAULT_FRONTEND_PORT = 7391

function parseEnvFile(content: string): Record<string, string> {
  const values: Record<string, string> = {}
  for (const line of content.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) {
      continue
    }
    const normalized = trimmed.startsWith('export ') ? trimmed.slice('export '.length).trim() : trimmed
    const eqIndex = normalized.indexOf('=')
    if (eqIndex <= 0) {
      continue
    }
    const key = normalized.slice(0, eqIndex).trim()
    let value = normalized.slice(eqIndex + 1).trim()
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1)
    }
    values[key] = value
  }
  return values
}

function loadSlimeBotEnv(): Record<string, string> {
  const configHome = process.env.SLIMEBOT_HOME || join(homedir(), '.slimebot')
  const envPath = join(configHome, 'config.cfg')
  if (!existsSync(envPath)) {
    return {}
  }
  return parseEnvFile(readFileSync(envPath, 'utf-8'))
}

function readPort(key: string, fallback: string | number): string {
  const fileEnv = loadSlimeBotEnv()
  const value = process.env[key] || fileEnv[key]
  return value && value.trim() ? value.trim() : String(fallback)
}

const serverPort = readPort('SERVER_PORT', DEFAULT_SERVER_PORT)
const frontendPort = Number.parseInt(readPort('FRONTEND_PORT', DEFAULT_FRONTEND_PORT), 10) || DEFAULT_FRONTEND_PORT

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  define: {
    __APP_VERSION__: JSON.stringify(packageJson.version || '0.0.0'),
  },
  build: {
    outDir: '../web/dist',
    emptyOutDir: true,
  },
  server: {
    host: '0.0.0.0',
    allowedHosts: true,
    fs: {
      allow: [searchForWorkspaceRoot(__dirname), path.resolve(__dirname, '../shared/chat-protocol')],
    },
    port: frontendPort,
    proxy: {
      '/api': { target: `http://localhost:${serverPort}`, changeOrigin: true },
      '/ws': { target: `ws://localhost:${serverPort}`, ws: true },
    },
  },
})
