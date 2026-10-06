import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { PanelLeft } from 'lucide-react';
import { useIsMobile } from '@/hooks/use-is-mobile';
import { cn } from '@/lib/utils';
import { Button } from './button';
import { Separator } from './separator';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from './sheet';
import { TooltipProvider } from './tooltip';

interface SidebarContextValue {
  isMobile: boolean;
  openMobile: boolean;
  setOpenMobile: React.Dispatch<React.SetStateAction<boolean>>;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
  sidebarId: string;
}

const SidebarContext = React.createContext<SidebarContextValue | null>(null);

export function useSidebar() {
  const context = React.useContext(SidebarContext);
  if (!context) throw new Error('useSidebar must be used within a SidebarProvider.');
  return context;
}

export function SidebarProvider({ className, style, children, ...props }: React.ComponentProps<'div'>) {
  const isMobile = useIsMobile();
  const [openMobile, setOpenMobile] = React.useState(false);
  const triggerRef = React.useRef<HTMLButtonElement>(null);
  const sidebarId = React.useId();

  React.useEffect(() => {
    if (!isMobile) setOpenMobile(false);
  }, [isMobile]);

  const context = React.useMemo<SidebarContextValue>(
    () => ({ isMobile, openMobile, setOpenMobile, triggerRef, sidebarId }),
    [isMobile, openMobile, sidebarId],
  );

  return (
    <SidebarContext.Provider value={context}>
      <TooltipProvider delayDuration={0}>
        <div
          data-slot="sidebar-wrapper"
          className={cn('ui-sidebar-provider flex min-h-dvh w-full bg-background text-foreground', className)}
          style={{ '--sidebar-width': '228px', ...style } as React.CSSProperties}
          {...props}
        >
          {children}
        </div>
      </TooltipProvider>
    </SidebarContext.Provider>
  );
}

type SidebarProps = React.ComponentProps<'div'> & {
  onCloseAutoFocus?: React.ComponentProps<typeof SheetContent>['onCloseAutoFocus'];
};

export function Sidebar({ className, children, onCloseAutoFocus, ...props }: SidebarProps) {
  const { isMobile, openMobile, setOpenMobile, triggerRef, sidebarId } = useSidebar();

  if (isMobile) {
    return (
      <Sheet open={openMobile} onOpenChange={setOpenMobile}>
        <SheetContent
          {...props}
          id={sidebarId}
          side="left"
          data-sidebar="sidebar"
          data-mobile="true"
          className={cn('ui-sidebar ui-sidebar-mobile gap-0 [&_[data-slot=sidebar-header]]:pr-14', className)}
          onCloseAutoFocus={(event) => {
            onCloseAutoFocus?.(event);
            const trigger = triggerRef.current;
            if (!event.defaultPrevented && trigger?.isConnected && trigger.getClientRects().length > 0) {
              event.preventDefault();
              trigger.focus();
            }
          }}
        >
          <SheetHeader className="sr-only">
            <SheetTitle>工作台导航</SheetTitle>
            <SheetDescription>选择可访问的工作台页面，按 Escape 或关闭按钮退出导航。</SheetDescription>
          </SheetHeader>
          {children}
        </SheetContent>
      </Sheet>
    );
  }

  return (
    <div
      {...props}
      id={sidebarId}
      data-slot="sidebar"
      data-sidebar="sidebar"
      data-mobile="false"
      className={cn('ui-sidebar flex w-(--sidebar-width) shrink-0 flex-col border-r border-border bg-popover text-popover-foreground', className)}
    >
      {children}
    </div>
  );
}

export const SidebarTrigger = React.forwardRef<HTMLButtonElement, React.ComponentPropsWithoutRef<typeof Button>>(
  ({ className, onClick, children, ...props }, ref) => {
    const { isMobile, openMobile, setOpenMobile, triggerRef, sidebarId } = useSidebar();
    const setTriggerRef = React.useCallback((node: HTMLButtonElement | null) => {
      triggerRef.current = node;
      if (typeof ref === 'function') return ref(node);
      if (ref) ref.current = node;
    }, [ref, triggerRef]);

    return (
      <Button
        {...props}
        ref={setTriggerRef}
        variant="ghost"
        size="icon"
        data-slot="sidebar-trigger"
        data-sidebar="trigger"
        aria-label={openMobile ? '关闭导航' : '打开导航'}
        aria-expanded={openMobile}
        aria-controls={sidebarId}
        className={cn('ui-sidebar-trigger min-h-11 min-w-11 min-[851px]:hidden', className)}
        onClick={(event) => {
          onClick?.(event);
          if (!event.defaultPrevented && isMobile) setOpenMobile((open) => !open);
        }}
      >
        {children ?? <PanelLeft size={18} aria-hidden="true" />}
      </Button>
    );
  },
);
SidebarTrigger.displayName = 'SidebarTrigger';

export function SidebarInset({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="sidebar-inset" className={cn('ui-sidebar-inset relative flex min-w-0 flex-1 flex-col', className)} {...props} />;
}

export function SidebarHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="sidebar-header" data-sidebar="header" className={cn('ui-sidebar-header flex shrink-0 flex-col gap-2 p-4', className)} {...props} />;
}

export function SidebarContent({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="sidebar-content" data-sidebar="content" className={cn('ui-sidebar-content flex min-h-0 flex-1 flex-col gap-2 overflow-auto', className)} {...props} />;
}

export function SidebarFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="sidebar-footer" data-sidebar="footer" className={cn('ui-sidebar-footer flex shrink-0 flex-col gap-2 p-4', className)} {...props} />;
}

export function SidebarGroup({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="sidebar-group" data-sidebar="group" className={cn('ui-sidebar-group relative flex w-full min-w-0 flex-col p-2', className)} {...props} />;
}

export function SidebarGroupLabel({ className, asChild = false, ...props }: React.ComponentProps<'div'> & { asChild?: boolean }) {
  const Component = asChild ? Slot : 'div';
  return <Component data-slot="sidebar-group-label" data-sidebar="group-label" className={cn('ui-sidebar-group-label flex min-h-11 shrink-0 items-center gap-2 rounded-md px-3 text-xs font-semibold text-muted-foreground [&>svg]:size-4 [&>svg]:shrink-0', className)} {...props} />;
}

export function SidebarGroupContent({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="sidebar-group-content" data-sidebar="group-content" className={cn('ui-sidebar-group-content w-full text-sm', className)} {...props} />;
}

export function SidebarMenu({ className, ...props }: React.ComponentProps<'ul'>) {
  return <ul data-slot="sidebar-menu" data-sidebar="menu" className={cn('ui-sidebar-menu flex w-full min-w-0 flex-col gap-1', className)} {...props} />;
}

export function SidebarMenuItem({ className, ...props }: React.ComponentProps<'li'>) {
  return <li data-slot="sidebar-menu-item" data-sidebar="menu-item" className={cn('ui-sidebar-menu-item relative', className)} {...props} />;
}

export function SidebarMenuButton({ asChild = false, isActive = false, className, type, ...props }: React.ComponentProps<'button'> & { asChild?: boolean; isActive?: boolean }) {
  const Component = asChild ? Slot : 'button';
  return (
    <Component
      data-slot="sidebar-menu-button"
      data-sidebar="menu-button"
      data-active={isActive}
      type={asChild ? type : type ?? 'button'}
      className={cn('ui-sidebar-menu-button flex min-h-11 w-full items-center gap-3 rounded-md px-3 py-2 text-left text-sm font-medium hover:bg-secondary hover:text-secondary-foreground data-[active=true]:bg-secondary data-[active=true]:text-secondary-foreground disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50 [&>svg]:size-4 [&>svg]:shrink-0', className)}
      {...props}
    />
  );
}

export function SidebarSeparator({ className, ...props }: React.ComponentProps<typeof Separator>) {
  return <Separator data-sidebar="separator" className={cn('ui-sidebar-separator mx-4 w-auto', className)} {...props} />;
}
