// @vitest-environment jsdom
import { act, useState } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { UserAvatar } from './WorkbenchShell';

// ES2022 is the suite's supported type baseline; Promise.withResolvers is unavailable.
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(finish => { resolve = finish; }); return { promise, resolve }; }
function image(value: string) { return new Response(value, { headers: { 'Content-Type': 'image/png' } }); }

let container: HTMLDivElement;
let root: Root;
let blobs: Blob[];
let revoked: string[];
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  blobs = []; revoked = [];
  class BrowserURL extends URL {
    static createObjectURL(blob: Blob) { blobs.push(blob); return `blob:avatar-${blobs.length}`; }
    static revokeObjectURL(url: string) { revoked.push(url); }
  }
  vi.stubGlobal('URL', BrowserURL);
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('current-user avatar display lifecycle', () => {
  it('replaces cached avatar contents on a new revision and releases replaced and unmounted URLs', async () => {
    let stored = 'original avatar';
    let cached = stored;
    vi.stubGlobal('fetch', vi.fn(async (_path: string, init: RequestInit) => {
      if (init.cache === 'reload') cached = stored;
      return image(cached);
    }));
    await act(async () => root.render(<UserAvatar id={7} name="操作者" />));
    expect(await blobs[0].text()).toBe('original avatar');
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:avatar-1');
    stored = 'normalized new avatar';
    await act(async () => root.render(<UserAvatar id={7} name="操作者" revision={1} />));
    expect(await blobs[1].text()).toBe('normalized new avatar');
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:avatar-2');
    expect(revoked).toContain('blob:avatar-1');
    act(() => root.render(null));
    expect(revoked).toContain('blob:avatar-2');
  });

  it('discards a late prior revision without creating a retained URL or replacing the new image', async () => {
    const old = deferred<Response>();
    const fetch = vi.fn().mockImplementationOnce(() => old.promise).mockResolvedValueOnce(image('new image'));
    vi.stubGlobal('fetch', fetch);
    await act(async () => root.render(<UserAvatar id={7} name="操作者" revision={1} />));
    await act(async () => root.render(<UserAvatar id={7} name="操作者" revision={2} />));
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:avatar-1');
    await act(async () => { old.resolve(image('obsolete image')); });
    expect(blobs).toHaveLength(1);
    expect(await blobs[0].text()).toBe('new image');
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:avatar-1');
    expect((fetch.mock.calls[0][1] as RequestInit).signal?.aborted).toBe(true);
  });

  it('lets a consumer distinguish image read failure and retry display without issuing another write', async () => {
    function Display({ revision }: { revision: number }) {
      const [status, setStatus] = useState('loading');
      return <><UserAvatar id={7} name="操作者" revision={revision} onLoad={() => setStatus('ready')} onError={() => setStatus('error')} /><output>{status}</output></>;
    }
    const fetch = vi.fn().mockRejectedValueOnce(new TypeError('offline')).mockResolvedValueOnce(image('saved server avatar'));
    vi.stubGlobal('fetch', fetch);
    await act(async () => root.render(<Display revision={1} />));
    expect(container.querySelector('output')?.textContent).toBe('error');
    await act(async () => root.render(<Display revision={2} />));
    act(() => container.querySelector('img')!.dispatchEvent(new Event('load')));
    expect(container.querySelector('output')?.textContent).toBe('ready');
    expect(fetch.mock.calls).toHaveLength(2);
    expect(fetch.mock.calls.every(([, init]) => init.method === 'GET')).toBe(true);
  });
});
