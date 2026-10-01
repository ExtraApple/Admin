import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@/lib/utils';
export const buttonVariants = cva('ui-button', { variants: { variant: { default: 'ui-button-primary', destructive: 'ui-button-destructive', outline: 'ui-button-outline', secondary: 'ui-button-secondary', ghost: 'ui-button-ghost', link: 'ui-button-link' }, size: { default: '', sm: 'ui-button-sm', lg: 'ui-button-lg', icon: 'ui-button-icon' } }, defaultVariants: { variant: 'default', size: 'default' } });
export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> { asChild?: boolean }
export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(({ className, variant, size, asChild = false, type, ...props }, ref) => { const Component = asChild ? Slot : 'button'; return <Component ref={ref} className={cn(buttonVariants({ variant, size }), className)} type={asChild ? type : type ?? 'button'} {...props} />; });
Button.displayName = 'Button';
