/**
 * Tests for detectProxyBindHost / PROXY_BIND_HOST security behaviour.
 *
 * The credential proxy holds real Anthropic API keys.  It must NEVER bind to
 * 0.0.0.0 — that would expose authenticated API access to every host on the
 * local network.  The safe fallback is always 127.0.0.1.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { NetworkInterfaceInfo } from 'os';

// Minimal mocks for modules loaded by container-runtime.ts at import time
vi.mock('./logger.js', () => ({
  logger: { debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn() },
}));

vi.mock('child_process', () => ({ execSync: vi.fn() }));

import { _detectProxyBindHost } from './container-runtime.js';
import { logger } from './logger.js';

// Clear spy call histories before each test so module-init calls (from the
// PROXY_BIND_HOST constant computed at import time) don't bleed into tests.
beforeEach(() => {
  vi.clearAllMocks();
});

// ---- helpers ----------------------------------------------------------------

function makeIpv4(address: string): NetworkInterfaceInfo {
  return {
    address,
    netmask: '255.255.0.0',
    family: 'IPv4',
    mac: '00:00:00:00:00:00',
    internal: false,
    cidr: `${address}/16`,
  };
}

// ---- tests ------------------------------------------------------------------
// All tests inject their own deps — no module-level mock state to manage.

describe('detectProxyBindHost — platform detection', () => {
  it('returns 127.0.0.1 on macOS regardless of interfaces', () => {
    const result = _detectProxyBindHost({
      platform: () => 'darwin',
      networkInterfaces: () => ({}),
      fileExists: () => false,
    });
    expect(result).toBe('127.0.0.1');
  });

  it('returns 127.0.0.1 on WSL (WSLInterop present)', () => {
    const result = _detectProxyBindHost({
      platform: () => 'linux',
      networkInterfaces: () => ({}),
      fileExists: (p) => p === '/proc/sys/fs/binfmt_misc/WSLInterop',
    });
    expect(result).toBe('127.0.0.1');
    expect(logger.warn).not.toHaveBeenCalled();
  });

  it('returns docker0 bridge IP when the interface is present', () => {
    const result = _detectProxyBindHost({
      platform: () => 'linux',
      networkInterfaces: () => ({ docker0: [makeIpv4('172.17.0.1')] }),
      fileExists: () => false,
    });
    expect(result).toBe('172.17.0.1');
    expect(logger.warn).not.toHaveBeenCalled();
  });
});

describe('detectProxyBindHost — safe fallback (the security fix)', () => {
  it('returns 127.0.0.1 — NOT 0.0.0.0 — when docker0 is absent', () => {
    const host = _detectProxyBindHost({
      platform: () => 'linux',
      networkInterfaces: () => ({ eth0: [makeIpv4('192.168.1.100')] }),
      fileExists: () => false,
    });
    expect(host).toBe('127.0.0.1');
    expect(host).not.toBe('0.0.0.0');
  });

  it('logs a warning so operators know to set CREDENTIAL_PROXY_HOST', () => {
    _detectProxyBindHost({
      platform: () => 'linux',
      networkInterfaces: () => ({}),
      fileExists: () => false,
    });
    expect(logger.warn).toHaveBeenCalledWith(
      expect.stringContaining('docker0'),
    );
    expect(logger.warn).toHaveBeenCalledWith(
      expect.stringContaining('CREDENTIAL_PROXY_HOST'),
    );
  });
});
