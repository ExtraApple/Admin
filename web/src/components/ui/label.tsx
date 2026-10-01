import * as React from 'react';
import * as Primitive from '@radix-ui/react-label';
import { cn } from '@/lib/utils';
export const Label = React.forwardRef<React.ElementRef<typeof Primitive.Root>, React.ComponentPropsWithoutRef<typeof Primitive.Root>>(({ className, ...props }, ref) => <Primitive.Root ref={ref} className={cn('ui-label', className)} {...props} />);
Label.displayName = 'Label';
