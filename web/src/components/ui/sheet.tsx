import * as React from 'react';
import * as Primitive from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from './button';

export const Sheet = Primitive.Root;
export const SheetTrigger = Primitive.Trigger;
export const SheetClose = Primitive.Close;
export const SheetPortal = Primitive.Portal;

export const SheetOverlay = React.forwardRef<React.ElementRef<typeof Primitive.Overlay>, React.ComponentPropsWithoutRef<typeof Primitive.Overlay>>(
  ({ className, ...props }, ref) => (
    <Primitive.Overlay ref={ref} data-slot="sheet-overlay" className={cn('ui-sheet-overlay fixed inset-0 z-50 bg-foreground/50', className)} {...props} />
  ),
);
SheetOverlay.displayName = 'SheetOverlay';

export const SheetContent = React.forwardRef<React.ElementRef<typeof Primitive.Content>, React.ComponentPropsWithoutRef<typeof Primitive.Content> & { side?: 'left' | 'right' }>(
  ({ className, children, side = 'right', ...props }, ref) => (
    <SheetPortal>
      <SheetOverlay />
      <Primitive.Content
        ref={ref}
        data-slot="sheet-content"
        data-side={side}
        className={cn(
          'ui-sheet-content fixed inset-y-0 z-50 flex h-dvh w-[min(288px,calc(100vw-48px))] flex-col gap-4 bg-popover text-popover-foreground shadow-xl',
          side === 'left' ? 'left-0 border-r border-border' : 'right-0 border-l border-border',
          className,
        )}
        {...props}
      >
        {children}
        <SheetClose asChild>
          <Button variant="ghost" size="icon" aria-label="关闭导航" className="ui-sheet-close absolute right-3 top-3 min-h-11 min-w-11">
            <X size={18} aria-hidden="true" />
          </Button>
        </SheetClose>
      </Primitive.Content>
    </SheetPortal>
  ),
);
SheetContent.displayName = 'SheetContent';

export function SheetHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="sheet-header" className={cn('ui-sheet-header flex flex-col gap-2 p-4', className)} {...props} />;
}

export function SheetFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="sheet-footer" className={cn('ui-sheet-footer mt-auto flex flex-col gap-2 p-4', className)} {...props} />;
}

export const SheetTitle = React.forwardRef<React.ElementRef<typeof Primitive.Title>, React.ComponentPropsWithoutRef<typeof Primitive.Title>>(
  ({ className, ...props }, ref) => <Primitive.Title ref={ref} data-slot="sheet-title" className={cn('ui-sheet-title text-base font-semibold text-foreground', className)} {...props} />,
);
SheetTitle.displayName = 'SheetTitle';

export const SheetDescription = React.forwardRef<React.ElementRef<typeof Primitive.Description>, React.ComponentPropsWithoutRef<typeof Primitive.Description>>(
  ({ className, ...props }, ref) => <Primitive.Description ref={ref} data-slot="sheet-description" className={cn('ui-sheet-description text-sm text-muted-foreground', className)} {...props} />,
);
SheetDescription.displayName = 'SheetDescription';
