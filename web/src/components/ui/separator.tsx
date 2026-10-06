import * as React from 'react';
import * as Primitive from '@radix-ui/react-separator';
import { cn } from '@/lib/utils';

export const Separator = React.forwardRef<React.ElementRef<typeof Primitive.Root>, React.ComponentPropsWithoutRef<typeof Primitive.Root>>(
  ({ className, orientation = 'horizontal', decorative = true, ...props }, ref) => (
    <Primitive.Root
      ref={ref}
      data-slot="separator"
      decorative={decorative}
      orientation={orientation}
      className={cn('ui-separator shrink-0 bg-border data-[orientation=horizontal]:h-px data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-px', className)}
      {...props}
    />
  ),
);
Separator.displayName = 'Separator';
