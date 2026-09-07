'use strict'

const mc = require('minecraft-protocol')

const host = process.env.CYU_SMOKE_HOST || '127.0.0.1'
const port = Number(process.env.CYU_SMOKE_PORT || '25577')
const timeoutMs = Number(process.env.CYU_SMOKE_TIMEOUT_MS || '20000')

const required = new Set(['login', 'position', 'map_chunk'])
const seen = new Set()
let finished = false

function finish (code, message) {
  if (finished) return
  finished = true
  clearTimeout(timeout)
  if (message) {
    const stream = code === 0 ? process.stdout : process.stderr
    stream.write(message + '\n')
  }
  try { client.end() } catch {}
  setTimeout(() => process.exit(code), 25)
}

const client = mc.createClient({
  host,
  port,
  username: 'CyuSmoke',
  auth: 'offline',
  version: '26.2'
})

client.on('packet', (data, meta) => {
  if (!required.has(meta.name)) return
  seen.add(meta.name)
  process.stdout.write(`[smoke] received ${meta.name}\n`)

  if (meta.name === 'position' && Number.isInteger(data.teleportId)) {
    client.write('teleport_confirm', { teleportId: data.teleportId })
  }

  if ([...required].every(name => seen.has(name))) {
    finish(0, '[smoke] CyuCore reached Play with login, position and chunk decoded')
  }
})

client.on('error', err => {
  finish(1, `[smoke] protocol error: ${err && err.stack ? err.stack : err}`)
})

client.on('end', reason => {
  if (!finished) {
    finish(1, `[smoke] connection ended before Play smoke completed: ${reason || 'unknown reason'}; seen=${[...seen].join(',')}`)
  }
})

const timeout = setTimeout(() => {
  finish(1, `[smoke] timeout after ${timeoutMs}ms; seen=${[...seen].join(',')}`)
}, timeoutMs)
