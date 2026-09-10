#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const os = require('os');
const https = require('https');
const { spawn, execSync } = require('child_process');

const pkg = require('../package.json');
const VERSION = pkg.version;
const REPO = 'herliansyah/sqlfox';

function getTargetInfo() {
  const platformMap = {
    win32: 'windows',
    darwin: 'darwin',
    linux: 'linux'
  };

  const archMap = {
    x64: 'amd64',
    arm64: 'arm64'
  };

  const platform = platformMap[process.platform];
  const arch = archMap[process.arch];

  if (!platform || !arch) {
    console.error(`❌ Unsupported platform or architecture: ${process.platform}-${process.arch}`);
    console.error(`SqlFox prebuilt binaries are available for linux (amd64, arm64), darwin (amd64, arm64), and windows (amd64, arm64).`);
    process.exit(1);
  }

  const isWindows = platform === 'windows';
  const binaryName = isWindows ? 'sqlfox.exe' : 'sqlfox';
  const archiveExt = isWindows ? 'zip' : 'tar.gz';
  const archiveName = `sqlfox-v${VERSION}-${platform}-${arch}.${archiveExt}`;
  const downloadUrl = `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`;

  return {
    platform,
    arch,
    isWindows,
    binaryName,
    archiveName,
    downloadUrl
  };
}

function fetchWithRedirect(url) {
  return new Promise((resolve, reject) => {
    https.get(url, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        fetchWithRedirect(res.headers.location).then(resolve).catch(reject);
        return;
      }
      if (res.statusCode !== 200) {
        reject(new Error(`Failed to download: HTTP ${res.statusCode} from ${url}`));
        return;
      }
      resolve(res);
    }).on('error', reject);
  });
}

async function downloadFile(url, destPath) {
  const stream = await fetchWithRedirect(url);
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(destPath);
    stream.pipe(file);
    file.on('finish', () => {
      file.close(resolve);
    });
    file.on('error', (err) => {
      fs.unlink(destPath, () => reject(err));
    });
  });
}

function extractArchive(archivePath, extractDir, isWindows) {
  if (isWindows) {
    try {
      // Use built-in tar on modern Windows (10 build 17063+)
      execSync(`tar -xf "${archivePath}" -C "${extractDir}"`, { stdio: 'ignore' });
      return;
    } catch {
      // Fallback to PowerShell Expand-Archive
      execSync(`powershell -NoProfile -NonInteractive -Command "Expand-Archive -LiteralPath '${archivePath}' -DestinationPath '${extractDir}' -Force"`, { stdio: 'ignore' });
      return;
    }
  }

  // Unix / macOS
  execSync(`tar -xzf "${archivePath}" -C "${extractDir}"`, { stdio: 'ignore' });
}

async function ensureBinary() {
  // 1. Explicit override via env variable
  if (process.env.SQLFOX_BIN && fs.existsSync(process.env.SQLFOX_BIN)) {
    return process.env.SQLFOX_BIN;
  }

  const { platform, arch, isWindows, binaryName, archiveName, downloadUrl } = getTargetInfo();

  // 2. In-repo root binary during local development
  const localRepoBinary = path.resolve(__dirname, '..', binaryName);
  if (fs.existsSync(localRepoBinary)) {
    return localRepoBinary;
  }

  // 3. User cache directory: ~/.sqlfox/bin/sqlfox-vX.Y.Z-<os>-<arch>[.exe]
  const cacheDir = path.join(os.homedir(), '.sqlfox', 'bin');
  const cachedBinary = path.join(cacheDir, `sqlfox-v${VERSION}-${platform}-${arch}${isWindows ? '.exe' : ''}`);

  if (fs.existsSync(cachedBinary)) {
    return cachedBinary;
  }

  fs.mkdirSync(cacheDir, { recursive: true });

  console.log(`\n🦊 SqlFox ${VERSION}: Initializing for ${platform}-${arch}...`);
  console.log(`⬇️  Downloading prebuilt binary from GitHub Release...`);

  const tempArchive = path.join(cacheDir, archiveName);
  const tempExtractDir = path.join(cacheDir, `tmp-extract-${Date.now()}`);
  fs.mkdirSync(tempExtractDir, { recursive: true });

  try {
    await downloadFile(downloadUrl, tempArchive);
    extractArchive(tempArchive, tempExtractDir, isWindows);

    const extractedBinary = path.join(tempExtractDir, binaryName);
    if (!fs.existsSync(extractedBinary)) {
      throw new Error(`Binary ${binaryName} not found inside extracted archive.`);
    }

    fs.copyFileSync(extractedBinary, cachedBinary);
    if (!isWindows) {
      fs.chmodSync(cachedBinary, 0o755);
    }
    console.log(`✅ SqlFox ready at ${cachedBinary}\n`);
    return cachedBinary;
  } catch (err) {
    throw new Error(
      `Failed to download SqlFox binary: ${err.message}\n` +
      `URL: ${downloadUrl}\n` +
      `You can build from source using: go install github.com/${REPO}/cmd/sqlfox@latest`
    );
  } finally {
    // Cleanup temporary files
    try {
      if (fs.existsSync(tempArchive)) fs.unlinkSync(tempArchive);
      if (fs.existsSync(tempExtractDir)) fs.rmSync(tempExtractDir, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  }
}

async function main() {
  try {
    const binaryPath = await ensureBinary();
    const child = spawn(binaryPath, process.argv.slice(2), {
      stdio: 'inherit'
    });

    child.on('exit', (code, signal) => {
      if (signal) {
        process.kill(process.pid, signal);
      } else {
        process.exit(code || 0);
      }
    });

    const forwardSignal = (sig) => {
      if (!child.killed) child.kill(sig);
    };

    process.on('SIGINT', () => forwardSignal('SIGINT'));
    process.on('SIGTERM', () => forwardSignal('SIGTERM'));
  } catch (err) {
    console.error(`\n❌ ${err.message}\n`);
    process.exit(1);
  }
}

main();
