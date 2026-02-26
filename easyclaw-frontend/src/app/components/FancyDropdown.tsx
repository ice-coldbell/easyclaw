import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";
import { cn } from "./ui/utils";

export interface FancyDropdownOption {
  value: string;
  label: string;
  description?: string;
}

interface FancyDropdownProps {
  value: string;
  options: FancyDropdownOption[];
  onValueChange: (value: string) => void;
  ariaLabel: string;
  triggerClassName?: string;
  contentClassName?: string;
}

export function FancyDropdown({
  value,
  options,
  onValueChange,
  ariaLabel,
  triggerClassName,
  contentClassName,
}: FancyDropdownProps) {
  return (
    <Select value={value} onValueChange={onValueChange}>
      <SelectTrigger
        aria-label={ariaLabel}
        size="sm"
        className={cn(
          "h-9 min-w-[120px] rounded-lg border-border/70 bg-background/60 px-3 text-[0.75rem] text-foreground shadow-sm transition-colors hover:bg-background whitespace-nowrap",
          triggerClassName,
        )}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        className={cn(
          "rounded-xl border-border/80 bg-card/95 p-1 shadow-xl backdrop-blur-xl",
          contentClassName,
        )}
      >
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value} className="rounded-lg py-2 text-[0.75rem]">
            {option.description ? (
              <span className="flex flex-col">
                <span>{option.label}</span>
                <span className="text-[0.6875rem] text-muted-foreground">{option.description}</span>
              </span>
            ) : (
              <span className="whitespace-nowrap">{option.label}</span>
            )}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
