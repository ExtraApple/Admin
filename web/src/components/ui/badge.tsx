import type { HTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@/lib/utils';
export const badgeVariants = cva('ui-badge', { variants: { variant: { default: 'ui-badge-default', secondary: 'ui-badge-secondary', destructive: 'ui-badge-destructive', outline: 'ui-badge-outline' } }, defaultVariants: { variant: 'default' } });
export interface BadgeProps extends HTMLAttributes<HTMLSpanElement>, VariantProps<typeof badgeVariants> {}
export function Badge({ className, variant, ...props }: BadgeProps) { return <span className={cn(badgeVariants({ variant }), className)} {...props} />; }
