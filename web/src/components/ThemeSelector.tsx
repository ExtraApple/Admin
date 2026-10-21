import { ChevronDown, Laptop, Moon, Sun, type LucideIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { isThemeMode, THEME_MODES, useTheme, type ThemeMode } from '@/lib/theme';

const THEME_LABELS: Record<ThemeMode, string> = { light: '亮色', dark: '暗色', system: '跟随系统' };
const THEME_ICONS: Record<ThemeMode, LucideIcon> = { light: Sun, dark: Moon, system: Laptop };

export function ThemeSelector({ className }: { className?: string }) {
  const { mode, appearance, setMode } = useTheme();
  const SelectedIcon = THEME_ICONS[mode];
  const selectedLabel = THEME_LABELS[mode];
  const actualLabel = appearance === 'dark' ? '当前暗色' : '当前亮色';

  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button variant="ghost" className={cn('theme-selector', className)} aria-label={`外观：${selectedLabel}`} title={`当前外观：${selectedLabel}`}>
        <SelectedIcon className="theme-selector-icon" size={17} aria-hidden="true" />
        <span className="theme-selector-current">{selectedLabel}</span>
        <ChevronDown size={15} aria-hidden="true" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" className="theme-selector-menu">
      <DropdownMenuLabel>选择外观 · {mode === 'system' ? actualLabel : `固定${selectedLabel}`}</DropdownMenuLabel>
      <DropdownMenuRadioGroup value={mode} onValueChange={value => { if (isThemeMode(value)) setMode(value); }}>
        {THEME_MODES.map(option => {
          const Icon = THEME_ICONS[option];
          return <DropdownMenuRadioItem key={option} value={option} className="theme-selector-option">
            <Icon className="theme-selector-option-icon" size={16} aria-hidden="true" />
            <span>{THEME_LABELS[option]}</span>
          </DropdownMenuRadioItem>;
        })}
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>;
}
