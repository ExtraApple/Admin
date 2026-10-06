import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';

export function Breadcrumb({ className, ...props }: React.ComponentProps<'nav'>) {
  return <nav aria-label="面包屑导航" data-slot="breadcrumb" className={cn('ui-breadcrumb min-w-0', className)} {...props} />;
}

export function BreadcrumbList({ className, ...props }: React.ComponentProps<'ol'>) {
  return <ol data-slot="breadcrumb-list" className={cn('ui-breadcrumb-list flex flex-wrap items-center gap-2 text-xs text-muted-foreground break-words', className)} {...props} />;
}

export function BreadcrumbItem({ className, ...props }: React.ComponentProps<'li'>) {
  return <li data-slot="breadcrumb-item" className={cn('ui-breadcrumb-item inline-flex min-w-0 items-center gap-2', className)} {...props} />;
}

export function BreadcrumbLink({ asChild = false, className, ...props }: React.ComponentProps<'a'> & { asChild?: boolean }) {
  const Component = asChild ? Slot : 'a';
  return <Component data-slot="breadcrumb-link" className={cn('ui-breadcrumb-link inline-flex min-h-11 items-center hover:text-foreground', className)} {...props} />;
}

export function BreadcrumbPage({ className, ...props }: React.ComponentProps<'span'>) {
  return <span data-slot="breadcrumb-page" role="link" aria-disabled="true" aria-current="page" className={cn('ui-breadcrumb-page font-semibold text-foreground', className)} {...props} />;
}

export function BreadcrumbSeparator({ children, className, ...props }: React.ComponentProps<'li'>) {
  return (
    <li data-slot="breadcrumb-separator" role="presentation" aria-hidden="true" className={cn('ui-breadcrumb-separator [&>svg]:size-3.5', className)} {...props}>
      {children ?? <ChevronRight />}
    </li>
  );
}
