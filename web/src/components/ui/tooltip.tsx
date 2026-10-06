import * as React from 'react';
import * as Primitive from '@radix-ui/react-tooltip';
import { cn } from '@/lib/utils';

export function TooltipProvider({ delayDuration = 0, ...props }: React.ComponentProps<typeof Primitive.Provider>) {
  return <Primitive.Provider delayDuration={delayDuration} {...props} />;
}

export const Tooltip = Primitive.Root;
export const TooltipTrigger = Primitive.Trigger;

export const TooltipContent = React.forwardRef<React.ElementRef<typeof Primitive.Content>, React.ComponentPropsWithoutRef<typeof Primitive.Content>>(
  ({ className, children, sideOffset = 4, ...props }, ref) => (
    <Primitive.Portal>
      <Primitive.Content
        ref={ref}
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        className={cn('ui-tooltip-content z-50 max-w-[calc(100vw-32px)] rounded-md bg-foreground px-3 py-2 text-xs text-background shadow-md', className)}
        {...props}
      >
        {children}
        <Primitive.Arrow className="fill-foreground" />
      </Primitive.Content>
    </Primitive.Portal>
  ),
);
TooltipContent.displayName = 'TooltipContent';
