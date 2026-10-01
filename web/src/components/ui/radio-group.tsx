import * as React from 'react';
import * as Primitive from '@radix-ui/react-radio-group';
import { cn } from '@/lib/utils';
export const RadioGroup = React.forwardRef<React.ElementRef<typeof Primitive.Root>, React.ComponentPropsWithoutRef<typeof Primitive.Root>>(({ className, ...props }, ref) => <Primitive.Root ref={ref} className={cn('ui-radio-group', className)} {...props} />);
export const RadioGroupItem = React.forwardRef<React.ElementRef<typeof Primitive.Item>, React.ComponentPropsWithoutRef<typeof Primitive.Item>>(({ className, ...props }, ref) => <Primitive.Item ref={ref} className={cn('ui-radio', className)} {...props}><Primitive.Indicator className="ui-radio-dot" /></Primitive.Item>);
RadioGroup.displayName = 'RadioGroup'; RadioGroupItem.displayName = 'RadioGroupItem';
