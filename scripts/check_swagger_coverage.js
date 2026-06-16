#!/usr/bin/env node
const fs = require('fs')
const path = require('path')

const repoRoot = path.resolve(__dirname, '..')
const routesText = fs.readFileSync(path.join(repoRoot, 'server/internal/routes/routes.go'), 'utf8')
const swagger = JSON.parse(fs.readFileSync(path.join(repoRoot, 'server/docs/swagger.json'), 'utf8'))

const groups = new Map([
  ['router', ''],
])
const routes = new Set()

function joinPath(base, suffix) {
  const joined = `${base || ''}/${suffix || ''}`.replace(/\/+/g, '/')
  return joined !== '/' ? joined.replace(/\/$/, '') : '/'
}

function toSwaggerPath(routePath) {
  let p = routePath.replace(/:([A-Za-z0-9_]+)/g, '{$1}')
  if (p.startsWith('/api/')) p = p.slice('/api'.length)
  if (p === '/api') p = '/'
  return p
}

for (const line of routesText.split(/\r?\n/)) {
  const groupMatch = line.match(/\b(\w+)\s*:=\s*(\w+)\.Group\("([^"]*)"\)/)
  if (groupMatch) {
    const [, name, parent, suffix] = groupMatch
    if (groups.has(parent)) {
      groups.set(name, joinPath(groups.get(parent), suffix))
    }
    continue
  }

  const routeMatch = line.match(/\b(\w+)\.(GET|POST|PUT|DELETE)\("([^"]*)"/)
  if (!routeMatch) continue

  const [, groupName, method, suffix] = routeMatch
  if (!groups.has(groupName)) continue

  const fullPath = joinPath(groups.get(groupName), suffix)
  if (!fullPath.startsWith('/api')) continue
  if (fullPath.startsWith('/api/swagger')) continue

  routes.add(`${method.toLowerCase()} ${toSwaggerPath(fullPath)}`)
}

const documented = new Set()
for (const [routePath, methods] of Object.entries(swagger.paths || {})) {
  for (const method of Object.keys(methods)) {
    documented.add(`${method.toLowerCase()} ${routePath}`)
  }
}

const missing = [...routes].filter(route => !documented.has(route)).sort()
const coverage = routes.size === 0 ? 100 : ((routes.size - missing.length) / routes.size) * 100

console.log(`Swagger route coverage: ${coverage.toFixed(1)}% (${routes.size - missing.length}/${routes.size})`)
if (missing.length) {
  console.log('Missing routes:')
  for (const route of missing) console.log(`  ${route}`)
}

if (coverage < 90) {
  process.exit(1)
}
