import * as React from 'react';
import * as Primitive from '@radix-ui/react-checkbox';
import { Check, Minus } from 'lucide-react';
import { cn } from '@/lib/utils';
export const Checkbox = React.forwardRef<React.ElementRef<typeof Primitive.Root>, React.ComponentPropsWithoutRef<typeof Primitive.Root>>(({ className, checked, ...props }, ref) => <Primitive.Root ref={ref} checked={checked} className={cn('ui-checkbox', className)} {...props}><Primitive.Indicator>{checked === 'indeterminate' ? <Minus size={15} /> : <Check size={15} />}</Primitive.Indicator></Primitive.Root>);
Checkbox.displayName = 'Checkbox';
