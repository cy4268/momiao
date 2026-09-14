import { spawnSync } from 'node:child_process'
import { lstat, mkdir, readFile, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const platformRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const nativeWeb = path.resolve(process.argv[2] || path.join(platformRoot, '..', 'native', 'web'))
const output = path.join(platformRoot, 'native-auth-web')

if (process.argv.length > 3) {
  throw new Error('output is fixed at platform/native-auth-web')
}

const runner = path.join(nativeWeb, 'node_modules', '@rsbuild', 'core', 'bin', 'rsbuild.js')
if (!(await stat(runner)).isFile()) {
  throw new Error(`Native web dependencies are unavailable: ${runner}`)
}

try {
  const existing = await lstat(output)
  if (existing.isSymbolicLink()) {
    throw new Error('platform/native-auth-web must not be a symlink or junction')
  }
  throw new Error('platform/native-auth-web already exists; rotate the verified generated directory before rebuilding')
} catch (error) {
  if (!(error && typeof error === 'object' && 'code' in error && error.code === 'ENOENT')) {
    throw error
  }
}

await mkdir(output)

const build = spawnSync(process.execPath, [runner, 'build'], {
  cwd: nativeWeb,
  env: {
    ...process.env,
    MOMIAO_AUTH_WEB_BUILD: '1',
    MOMIAO_AUTH_WEB_OUTPUT: output,
  },
  encoding: 'utf8',
  stdio: 'inherit',
})
if (build.error) throw build.error
if (build.status !== 0) process.exit(build.status ?? 1)

const index = await readFile(path.join(output, 'index.html'), 'utf8')
if (!index.includes('/native-auth/static/')) {
  throw new Error('Native auth build did not use the isolated /native-auth/ asset prefix')
}

process.stdout.write(`Native auth web build: ${output}\n`)
