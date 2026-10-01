import * as React from 'react';
import { cn } from '@/lib/utils';
export const Alert = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement> & { variant?: 'default' | 'destructive' }>(({ className, variant = 'default', ...props }, ref) => <div ref={ref} role="alert" className={cn('ui-alert', variant === 'destructive' && 'ui-alert-destructive', className)} {...props} />);
export const AlertTitle = ({ className, ...props }: React.HTMLAttributes<HTMLHeadingElement>) => <h3 className={cn('ui-alert-title', className)} {...props} />;
export const AlertDescription = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => <div className={cn('ui-alert-description', className)} {...props} />;
Alert.displayName = 'Alert';
