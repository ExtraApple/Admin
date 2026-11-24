import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronDown, Pin, X } from 'lucide-react';
import { canCloseTab, canPinTab, getBatchCloseState, type WorkbenchTab, type WorkbenchTabKey } from '@/lib/workbench-tabs';
import { cn } from '@/lib/utils';
import { Button } from './ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from './ui/dropdown-menu';
import { useSidebar } from './ui/sidebar';

export interface WorkbenchTabBarProps {
  tabs: WorkbenchTab[];
  activeKey: WorkbenchTabKey | null;
  onSelectTab?: () => void;
  onCloseTab: (key: WorkbenchTabKey) => void;
  onBatchClose: (anchorKey: WorkbenchTabKey, mode: 'others' | 'right') => void;
  onTogglePin: (key: WorkbenchTabKey) => void;
}

export function WorkbenchTabBar({ tabs, activeKey, onSelectTab, onCloseTab, onBatchClose, onTogglePin }: WorkbenchTabBarProps) {
  const { isMobile } = useSidebar();
  const [contextTabKey, setContextTabKey] = useState<WorkbenchTabKey | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const activeIndex = activeKey ? tabs.findIndex(tab => tab.key === activeKey) : -1;
  const activeTab = activeIndex >= 0 ? tabs[activeIndex] : null;

  useEffect(() => {
    if (!activeKey || !scrollRef.current) return;
    const activeEl = scrollRef.current.querySelector<HTMLElement>(`[data-tab-key="${activeKey}"]`);
    activeEl?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  }, [activeKey, activeIndex, tabs.length]);
  useEffect(() => {
    if (contextTabKey && !tabs.some(tab => tab.key === contextTabKey)) {
      setContextTabKey(null);
    }
  }, [contextTabKey, tabs]);

  if (isMobile || tabs.length === 0) return null;

  const toolbarAnchorKey = activeKey ?? tabs[tabs.length - 1]?.key ?? null;
  const toolbarBatch = getBatchCloseState(tabs, toolbarAnchorKey);
  const canPinActive = canPinTab(activeTab);

  return (
    <nav className="workbench-tabbar" aria-label="已打开页面">
      <div className="workbench-tabbar-scroll" ref={scrollRef} role="list">
        {tabs.map(tab => {
          const active = tab.key === activeKey;
          const tabCloseable = canCloseTab(tabs, tab.key);
          const tabPinnable = canPinTab(tab);
          const tabBatch = getBatchCloseState(tabs, tab.key);
          return (
            <div
              key={tab.key}
              role="listitem"
              data-tab-key={tab.key}
              data-active={active ? 'true' : 'false'}
              data-pinned={tab.pinned ? 'true' : 'false'}
              className={cn('workbench-tab', active && 'workbench-tab-active', tab.pinned && 'workbench-tab-pinned')}
              onMouseDown={event => {
                if (event.button === 1 && tabCloseable) event.preventDefault();
              }}
              onAuxClick={event => {
                if (event.button === 1 && tabCloseable) {
                  event.preventDefault();
                  onCloseTab(tab.key);
                }
              }}
              onContextMenu={event => {
                event.preventDefault();
                setContextTabKey(tab.key);
              }}
            >
              <Link
                className="workbench-tab-link"
                to={tab.url}
                aria-current={active ? 'page' : undefined}
                title={tab.label}
                onClick={() => onSelectTab?.()}
              >
                {tab.pinned && <Pin size={12} className="workbench-tab-pin" aria-hidden="true" />}
                <span>{tab.label}</span>
              </Link>
              {tabCloseable && (
                <button
                  type="button"
                  className="workbench-tab-close"
                  aria-label={`关闭${tab.label}`}
                  title={`关闭${tab.label}`}
                  onClick={event => {
                    event.stopPropagation();
                    onCloseTab(tab.key);
                  }}
                >
                  <X size={13} aria-hidden="true" />
                </button>
              )}
              <DropdownMenu
                modal={false}
                open={contextTabKey === tab.key}
                onOpenChange={open => {
                  if (!open) setContextTabKey(current => (current === tab.key ? null : current));
                }}
              >
                <DropdownMenuTrigger asChild>
                  <span className="workbench-tab-menu-anchor" aria-hidden="true" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" side="bottom" sideOffset={4} className="workbench-tab-menu">
                  <DropdownMenuItem
                    disabled={!tabPinnable}
                    onSelect={() => {
                      setContextTabKey(null);
                      onTogglePin(tab.key);
                    }}
                  >
                    {tab.pinned ? '取消固定' : '固定页签'}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    disabled={!tabBatch.canCloseOthers}
                    onSelect={() => {
                      setContextTabKey(null);
                      onBatchClose(tab.key, 'others');
                    }}
                  >
                    关闭其他页签
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    disabled={!tabBatch.canCloseRight}
                    onSelect={() => {
                      setContextTabKey(null);
                      onBatchClose(tab.key, 'right');
                    }}
                  >
                    关闭右侧页签
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          );
        })}
      </div>
      <div className="workbench-tabbar-end">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="workbench-tabbar-actions"
              aria-label="页签操作"
              title="页签操作"
            >
              <ChevronDown size={15} aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="workbench-tab-menu">
            <DropdownMenuItem
              disabled={!canPinActive || !activeTab}
              onSelect={() => {
                if (activeTab) onTogglePin(activeTab.key);
              }}
            >
              {activeTab?.pinned ? '取消固定当前页签' : '固定当前页签'}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              disabled={!toolbarBatch.canCloseOthers || !toolbarAnchorKey}
              onSelect={() => {
                if (toolbarAnchorKey) onBatchClose(toolbarAnchorKey, 'others');
              }}
            >
              关闭其他页签
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={!toolbarBatch.canCloseRight || !toolbarAnchorKey}
              onSelect={() => {
                if (toolbarAnchorKey) onBatchClose(toolbarAnchorKey, 'right');
              }}
            >
              关闭右侧页签
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </nav>
  );
}
