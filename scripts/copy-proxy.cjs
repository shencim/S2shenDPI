#!/usr/bin/env node
/**
 * Copy darknes-proxy.exe from spoofdpi/ to src-tauri/binaries/
 * so Tauri sidecar finds it (dev + bundle).
 * Run after building the proxy: npm run build-proxy && npm run copy-proxy
 */
const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const src = path.join(root, 'spoofdpi', 'darknes-proxy.exe');
const destDir = path.join(root, 'src-tauri', 'binaries');
const destExe = path.join(destDir, 'darknes-proxy.exe');
const destTriple = path.join(destDir, 'darknes-proxy-x86_64-pc-windows-msvc.exe');

if (!fs.existsSync(src)) {
  console.error('spoofdpi/darknes-proxy.exe not found. Build it first: npm run build-proxy');
  process.exit(1);
}

if (!fs.existsSync(destDir)) {
  fs.mkdirSync(destDir, { recursive: true });
}

fs.copyFileSync(src, destExe);
fs.copyFileSync(src, destTriple);
console.log('Copied darknes-proxy.exe to src-tauri/binaries/ (and -x86_64-pc-windows-msvc.exe)');
