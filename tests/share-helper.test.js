import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Command } from 'commander';

const apiRequest = vi.fn();
const jsonOutput = vi.fn();
const jsonError = vi.fn((message) => { throw new Error(message); });

vi.mock('../src/lib/auth.js', () => ({
  loadConfig: () => ({ userId: 'u1' }),
  getValidToken: async () => 'token',
  wrapPayload: (_config, payload) => payload,
}));
vi.mock('../src/lib/http.js', () => ({ apiRequest }));
vi.mock('../src/lib/output.js', () => ({ jsonOutput, jsonError }));

const { registerShareHelper } = await import('../src/helpers/share.js');

function program() {
  const cmd = new Command();
  cmd.exitOverride();
  cmd.option('--verbose');
  registerShareHelper(cmd);
  return cmd;
}

describe('+share helper port parity', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('forwards the global --verbose flag to apiRequest', async () => {
    apiRequest.mockResolvedValue({ result: { data: { event: { title: 'Party' } } } });

    await program().parseAsync(['node', 'partiful', '--verbose', '+share', 'e1']);

    expect(apiRequest).toHaveBeenCalledWith(
      'POST',
      '/getEventInfo',
      'token',
      expect.any(Object),
      true,
    );
    expect(jsonOutput).toHaveBeenCalledWith({
      url: 'https://partiful.com/e/e1',
      eventId: 'e1',
      title: 'Party',
    });
  });

  it('preserves the JS fallback for an empty event title', async () => {
    apiRequest.mockResolvedValue({ result: { data: { event: { title: '' } } } });

    await program().parseAsync(['node', 'partiful', '+share', 'e1']);

    expect(jsonOutput).toHaveBeenCalledWith(expect.objectContaining({ title: 'Unknown Event' }));
  });
});
