// @vitest-environment jsdom
import { act, useState } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Sidebar, SidebarContent, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarTrigger, useSidebar } from './sidebar';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogTitle } from './alert-dialog';

function NavigationWithConfirmation() {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const { setOpenMobile } = useSidebar();
  return <>
    <Sidebar><SidebarContent><SidebarMenu><SidebarMenuItem><SidebarMenuButton asChild>
      <a href="/profile" data-navigation onClick={event => { event.preventDefault(); setConfirmOpen(true); }}>个人资料</a>
    </SidebarMenuButton></SidebarMenuItem></SidebarMenu></SidebarContent></Sidebar>
    <SidebarTrigger />
    <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
      <AlertDialogContent>
        <AlertDialogTitle>放弃未保存的修改？</AlertDialogTitle>
        <AlertDialogDescription>取消后继续编辑，确认后离开。</AlertDialogDescription>
        <AlertDialogCancel data-cancel>取消</AlertDialogCancel>
        <AlertDialogAction data-confirm onClick={() => setOpenMobile(false)}>放弃修改并离开</AlertDialogAction>
      </AlertDialogContent>
    </AlertDialog>
  </>;
}

describe('mobile navigation with a nested unsaved confirmation', () => {
  let root: Root;
  let container: HTMLDivElement;
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    vi.stubGlobal('matchMedia', (query: string) => ({ matches: query.includes('850px'), media: query, addEventListener() {}, removeEventListener() {} }));
    container = document.createElement('div');
    document.body.append(container);
    root = createRoot(container);
  });
  afterEach(async () => {
    await act(async () => { root.unmount(); await vi.runOnlyPendingTimersAsync(); });
    container.remove();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  async function settle() { await act(async () => { await vi.runOnlyPendingTimersAsync(); }); }
  function control(selector: string): HTMLElement {
    const element = document.querySelector<HTMLElement>(selector);
    if (!element) throw new Error(`Missing visible control: ${selector}`);
    return element;
  }
  async function click(selector: string) {
    act(() => { const element = control(selector); element.focus(); element.click(); });
    await settle();
  }

  it('keeps the Sheet usable after cancel and releases modal pointer lock after confirmed departure', async () => {
    act(() => root.render(<SidebarProvider><NavigationWithConfirmation /></SidebarProvider>));
    const trigger = control('.ui-sidebar-trigger');
    // jsdom has no layout; model only the trigger visibility required for real close-focus restoration.
    vi.spyOn(trigger, 'getClientRects').mockReturnValue([new DOMRect(0, 0, 44, 44)] as unknown as DOMRectList);
    await click('.ui-sidebar-trigger');
    expect(document.querySelector('[role="dialog"]')?.contains(document.activeElement)).toBe(true);

    await click('[data-navigation]');
    expect(document.querySelector('[role="alertdialog"]')?.contains(document.activeElement)).toBe(true);
    await click('[data-cancel]');
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
    expect(getComputedStyle(control('[role="dialog"]')).pointerEvents).not.toBe('none');

    await click('[data-navigation]');
    await click('[data-confirm]');
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.body.style.pointerEvents).not.toBe('none');
    expect(document.activeElement).toBe(trigger);
  });
});
